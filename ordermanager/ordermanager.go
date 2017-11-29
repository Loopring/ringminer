/*

  Copyright 2017 Loopring Project Ltd (Loopring Foundation).

  Licensed under the Apache License, Version 2.0 (the "License");
  you may not use this file except in compliance with the License.
  You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

  Unless required by applicable law or agreed to in writing, software
  distributed under the License is distributed on an "AS IS" BASIS,
  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
  See the License for the specific language governing permissions and
  limitations under the License.

*/

package ordermanager

import (
	"errors"
	"github.com/Loopring/relay/config"
	"github.com/Loopring/relay/dao"
	"github.com/Loopring/relay/ethaccessor"
	"github.com/Loopring/relay/eventemiter"
	"github.com/Loopring/relay/log"
	"github.com/Loopring/relay/marketcap"
	"github.com/Loopring/relay/types"
	"github.com/Loopring/relay/usermanager"
	"github.com/ethereum/go-ethereum/common"
	"math/big"
	"sync"
)

type OrderManager interface {
	Start()
	Stop()
	MinerOrders(tokenS, tokenB common.Address, filterOrderhashs []common.Hash) []types.OrderState
	GetOrderBook(protocol, tokenS, tokenB common.Address, length int) ([]types.OrderState, error)
	GetOrders(query *dao.Order, pageIndex, pageSize int) (dao.PageResult, error)
	GetOrderByHash(hash common.Hash) (types.OrderState, error)
	UpdateBroadcastTimeByHash(hash common.Hash, bt int) error
}

type OrderManagerImpl struct {
	options    config.OrderManagerOptions
	commonOpts *config.CommonOptions
	rds        dao.RdsService
	lock       sync.RWMutex
	processor  *forkProcessor
	provider   *minerOrdersProvider
	um         usermanager.UserManager
	mc         *marketcap.MarketCapProvider
}

func NewOrderManager(options config.OrderManagerOptions,
	commonOpts *config.CommonOptions,
	rds dao.RdsService,
	userManager usermanager.UserManager,
	accessor *ethaccessor.EthNodeAccessor) *OrderManagerImpl {

	om := &OrderManagerImpl{}
	om.options = options
	om.commonOpts = commonOpts
	om.rds = rds
	om.processor = newForkProcess(om.rds, accessor)
	om.um = userManager

	// new miner orders provider
	om.provider = newMinerOrdersProvider(options.TickerDuration, options.BlockPeriod, om.commonOpts, om.rds)

	return om
}

// Start start orderbook as a service
func (om *OrderManagerImpl) Start() {
	newOrderWatcher := &eventemitter.Watcher{Concurrent: false, Handle: om.handleGatewayOrder}
	ringMinedWatcher := &eventemitter.Watcher{Concurrent: false, Handle: om.handleRingMined}
	fillOrderWatcher := &eventemitter.Watcher{Concurrent: false, Handle: om.handleOrderFilled}
	cancelOrderWatcher := &eventemitter.Watcher{Concurrent: false, Handle: om.handleOrderCancelled}
	cutoffOrderWatcher := &eventemitter.Watcher{Concurrent: false, Handle: om.handleOrderCutoff}
	forkWatcher := &eventemitter.Watcher{Concurrent: false, Handle: om.handleFork}

	eventemitter.On(eventemitter.OrderManagerGatewayNewOrder, newOrderWatcher)
	eventemitter.On(eventemitter.OrderManagerExtractorRingMined, ringMinedWatcher)
	eventemitter.On(eventemitter.OrderManagerExtractorFill, fillOrderWatcher)
	eventemitter.On(eventemitter.OrderManagerExtractorCancel, cancelOrderWatcher)
	eventemitter.On(eventemitter.OrderManagerExtractorCutoff, cutoffOrderWatcher)
	eventemitter.On(eventemitter.OrderManagerFork, forkWatcher)

	om.provider.start()
}

func (om *OrderManagerImpl) Stop() {
	om.lock.Lock()
	defer om.lock.Unlock()

	om.provider.stop()
}

func (om *OrderManagerImpl) handleFork(input eventemitter.EventData) error {
	om.Stop()

	if err := om.processor.fork(input.(*ethaccessor.ForkedEvent)); err != nil {
		log.Errorf("order manager handle fork error:%s", err.Error())
	}

	om.Start()

	return nil
}

// 来自ipfs的新订单
// 所有来自ipfs的订单都是新订单
func (om *OrderManagerImpl) handleGatewayOrder(input eventemitter.EventData) error {
	om.lock.Lock()
	defer om.lock.Unlock()

	state := input.(*types.OrderState)
	state.Status = types.ORDER_NEW
	model := &dao.Order{}

	log.Debugf("ordermanager handle gateway order,order.hash:%s", state.RawOrder.Hash.Hex())
	log.Debugf("ordermanager handle gateway order,order.amounts:%s", state.RawOrder.AmountS.String())

	if err := model.ConvertDown(state); err != nil {
		log.Error(err.Error())
		return err
	}
	if err := om.rds.Add(model); err != nil {
		return err
	}

	return nil
}

func (om *OrderManagerImpl) handleRingMined(input eventemitter.EventData) error {
	event := input.(*types.RingMinedEvent)

	model, err := om.rds.FindRingMinedByRingHash(event.Ringhash.Hex())
	if err != nil {
		model = &dao.RingMined{}
		if err := model.ConvertDown(event); err != nil {
			return err
		}

		om.rds.Add(model)
	} else {
		modelConvertEvent := &types.RingMinedEvent{}
		if err := model.ConvertUp(modelConvertEvent); err != nil {
			return err
		}

		if modelConvertEvent.RingIndex.BigInt().Cmp(event.RingIndex.BigInt()) != 0 {
			om.rds.Add(model)
		}
	}

	return nil
}

func (om *OrderManagerImpl) handleOrderFilled(input eventemitter.EventData) error {
	event := input.(*types.OrderFilledEvent)

	// set miner order provider current block number
	om.provider.setBlockNumber(event.Blocknumber)

	// save event
	_, err := om.rds.FindFillEventByRinghashAndOrderhash(event.Ringhash, event.OrderHash)
	if err != nil {
		newFillModel := &dao.FillEvent{}
		if err := newFillModel.ConvertDown(event); err != nil {
			return err
		}
		if err := om.rds.Add(newFillModel); err != nil {
			return err
		}
	}

	// get rds.Order and types.OrderState
	state := &types.OrderState{}
	orderhash := event.OrderHash
	model, err := om.rds.GetOrderByHash(orderhash)
	if err != nil {
		return err
	}
	if err := model.ConvertUp(state); err != nil {
		return err
	}

	// judge status
	om.orderFullFinished(state)

	if state.Status == types.ORDER_CUTOFF || state.Status == types.ORDER_FINISHED || state.Status == types.ORDER_UNKNOWN {
		return errors.New("order manager handle order filled event error:order status is " + state.Status.Name())
	}

	// validate cutoff
	if cutoffModel, err := om.rds.FindCutoffEventByOwnerAddress(state.RawOrder.TokenS); err == nil {
		if beenCutoff := om.rds.CheckOrderCutoff(orderhash.Hex(), cutoffModel.Cutoff); beenCutoff {
			if err := om.rds.SettleOrdersStatus([]string{orderhash.Hex()}, types.ORDER_CUTOFF); err != nil {
				return err
			} else {
				return errors.New("order manager handle order filled event error:order have been cutoff")
			}
		}
	}

	// calculate orderState.remainAmounts
	state.BlockNumber = event.Blocknumber.BigInt()
	state.RemainedAmountS = event.AmountS.BigInt()
	if state.RawOrder.BuyNoMoreThanAmountB == true && event.AmountB.BigInt().Cmp(state.RawOrder.AmountB) > 0 {
		state.RemainedAmountB = state.RawOrder.AmountB
	} else {
		state.RemainedAmountB = event.AmountB.BigInt()
	}
	if event.AmountS.BigInt().Cmp(big.NewInt(0)) < 1 {
		state.RemainedAmountS = big.NewInt(0)
	}

	// update order status
	om.orderFullFinished(state)

	// update rds.Order
	if err := model.ConvertDown(state); err != nil {
		return err
	}
	if err := om.rds.Update(state); err != nil {
		return err
	}

	return nil
}

func (om *OrderManagerImpl) handleOrderCancelled(input eventemitter.EventData) error {
	event := input.(*types.OrderCancelledEvent)
	orderhash := event.OrderHash

	// set miner order provider current block number
	om.provider.setBlockNumber(event.Blocknumber)

	// save event
	_, err := om.rds.FindCancelEventByOrderhash(orderhash)
	if err != nil {
		newCancelEventModel := &dao.CancelEvent{}
		if err := newCancelEventModel.ConvertDown(event); err != nil {
			return err
		}
		if err := om.rds.Add(newCancelEventModel); err != nil {
			return err
		}
	}

	// get rds.Order and types.OrderState
	state := &types.OrderState{}
	model, err := om.rds.GetOrderByHash(orderhash)
	if err != nil {
		return err
	}
	if err := model.ConvertUp(state); err != nil {
		return err
	}

	// judge status
	om.orderFullFinished(state)
	if state.Status == types.ORDER_CUTOFF || state.Status == types.ORDER_FINISHED || state.Status == types.ORDER_UNKNOWN {
		return errors.New("order manager handle order filled event error:order status is " + state.Status.Name())
	}

	// calculate remainAmount
	if state.RawOrder.BuyNoMoreThanAmountB {
		state.RemainedAmountB = new(big.Int).Sub(state.RemainedAmountB, event.AmountCancelled.BigInt())
		if state.RemainedAmountB.Cmp(big.NewInt(0)) < 0 {
			log.Errorf("order:%s cancel amountB:%s error", orderhash.Hex(), event.AmountCancelled.BigInt().String())
			state.RemainedAmountB = big.NewInt(0)
		}
	} else {
		state.RemainedAmountS = new(big.Int).Sub(state.RemainedAmountS, event.AmountCancelled.BigInt())
		if state.RemainedAmountS.Cmp(big.NewInt(0)) < 0 {
			log.Errorf("order:%s cancel amountS:%s error", orderhash.Hex(), event.AmountCancelled.BigInt().String())
			state.RemainedAmountS = big.NewInt(0)
		}
	}

	// update order status
	om.orderFullFinished(state)

	// update rds.Order
	if err := model.ConvertDown(state); err != nil {
		return err
	}
	if err := om.rds.Update(state); err != nil {
		return err
	}

	return nil
}

func (om *OrderManagerImpl) handleOrderCutoff(input eventemitter.EventData) error {
	event := input.(*types.CutoffEvent)

	// set miner order provider current block number
	om.provider.setBlockNumber(event.Blocknumber)

	// save event
	model, err := om.rds.FindCutoffEventByOwnerAddress(event.Owner)
	if err != nil {
		model = &dao.CutOffEvent{}
		if err := model.ConvertDown(event); err != nil {
			return err
		}
		if err := om.rds.Add(model); err != nil {
			return err
		}
	} else {
		if err := model.ConvertDown(event); err != nil {
			return err
		}
		if err := om.rds.Update(model); err != nil {
			return err
		}
	}

	// get order list
	list, err := om.rds.GetCutoffOrders(model.Cutoff)
	if err != nil {
		return err
	}

	// update each order
	var orderhashs []string
	for _, order := range list {
		orderhashs = append(orderhashs, order.OrderHash)
	}
	if err := om.rds.SettleOrdersStatus(orderhashs, types.ORDER_CUTOFF); err != nil {
		return err
	}

	return nil
}

func (om *OrderManagerImpl) orderFullFinished(state *types.OrderState) {
	var sum *big.Rat

	if state.RawOrder.BuyNoMoreThanAmountB {
		ret := new(big.Int).Sub(state.RawOrder.AmountB, state.RemainedAmountB)
		price := om.mc.GetMarketCap(state.RawOrder.TokenB)
		remain := new(big.Rat).SetInt(ret)
		sum = new(big.Rat).Mul(price, remain)
	} else {
		price := om.mc.GetMarketCap(state.RawOrder.TokenS)
		remain := new(big.Rat).SetInt(state.RemainedAmountS)
		sum = new(big.Rat).Mul(price, remain)
	}

	// todo: get compare number from config
	if sum.Cmp(big.NewRat(10, 1)) <= 0 {
		state.Status = types.ORDER_FINISHED
	}
}

func (om *OrderManagerImpl) MinerOrders(tokenS, tokenB common.Address, filterOrderhashs []common.Hash) []types.OrderState {
	var list []types.OrderState

	if err := om.provider.markOrders(filterOrderhashs); err != nil {
		log.Debugf("get miner orders error:%s", err.Error())
	}

	filterList := om.provider.getOrders(tokenS, tokenB, filterOrderhashs)

	for _, v := range filterList {
		if !om.um.InWhiteList(v.RawOrder.TokenS) {
			list = append(list, v)
		}
	}

	return list
}

func (om *OrderManagerImpl) GetOrderBook(protocol, tokenS, tokenB common.Address, length int) ([]types.OrderState, error) {
	var list []types.OrderState
	models, err := om.rds.GetOrderBook(protocol, tokenS, tokenB, length)
	if err != nil {
		return list, err
	}

	for _, v := range models {
		var state types.OrderState
		if err := v.ConvertUp(&state); err != nil {
			continue
		}
		list = append(list, state)
	}

	return list, nil
}

func (om *OrderManagerImpl) GetOrders(query *dao.Order, pageIndex, pageSize int) (dao.PageResult, error) {
	var (
		pageRes dao.PageResult
	)
	tmp, err := om.rds.OrderPageQuery(query, pageIndex, pageSize)
	if err != nil {
		return pageRes, err
	}
	pageRes.PageIndex = tmp.PageIndex
	pageRes.PageSize = tmp.PageSize
	pageRes.Total = tmp.Total

	for _, v := range tmp.Data {
		var state types.OrderState
		model := v.(dao.Order)
		if err := model.ConvertUp(&state); err != nil {
			continue
		}
		pageRes.Data = append(pageRes.Data, state)
	}

	return pageRes, nil
}

func (om *OrderManagerImpl) GetOrderByHash(hash common.Hash) (orderState types.OrderState, err error) {
	order, err := om.rds.GetOrderByHash(hash)
	if err != nil {
		return
	}
	result := &types.OrderState{}
	order.ConvertUp(result)
	return *result, nil
}

func (om *OrderManagerImpl) UpdateBroadcastTimeByHash(hash common.Hash, bt int) error {
	return om.rds.UpdateBroadcastTimeByHash(hash.Str(), bt)
}