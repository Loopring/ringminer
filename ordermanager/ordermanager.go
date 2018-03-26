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
	"fmt"
	"github.com/Loopring/relay/config"
	"github.com/Loopring/relay/dao"
	"github.com/Loopring/relay/eventemiter"
	"github.com/Loopring/relay/log"
	"github.com/Loopring/relay/marketcap"
	"github.com/Loopring/relay/types"
	"github.com/Loopring/relay/usermanager"
	"github.com/ethereum/go-ethereum/common"
	"math/big"
)

type OrderManager interface {
	Start()
	Stop()
	MinerOrders(protocol, tokenS, tokenB common.Address, length int, startBlockNumber, endBlockNumber int64, filterOrderHashLists ...*types.OrderDelayList) []*types.OrderState
	GetOrderBook(protocol, tokenS, tokenB common.Address, length int) ([]types.OrderState, error)
	GetOrders(query map[string]interface{}, statusList []types.OrderStatus, pageIndex, pageSize int) (dao.PageResult, error)
	GetOrderByHash(hash common.Hash) (*types.OrderState, error)
	UpdateBroadcastTimeByHash(hash common.Hash, bt int) error
	FillsPageQuery(query map[string]interface{}, pageIndex, pageSize int) (dao.PageResult, error)
	RingMinedPageQuery(query map[string]interface{}, pageIndex, pageSize int) (dao.PageResult, error)
	IsOrderCutoff(protocol, owner, token1, token2 common.Address, validsince *big.Int) bool
	IsOrderFullFinished(state *types.OrderState) bool
	IsValueDusted(tokenAddress common.Address, value *big.Rat) bool
	SetOrderCancelled(state *types.OrderState)
	GetFrozenAmount(owner common.Address, token common.Address, statusSet []types.OrderStatus) (*big.Int, error)
	GetFrozenLRCFee(owner common.Address, statusSet []types.OrderStatus) (*big.Int, error)
}

type OrderManagerImpl struct {
	options            *config.OrderManagerOptions
	rds                dao.RdsService
	processor          *ForkProcessor
	um                 usermanager.UserManager
	mc                 marketcap.MarketCapProvider
	cutoffCache        *CutoffCache
	newOrderWatcher    *eventemitter.Watcher
	ringMinedWatcher   *eventemitter.Watcher
	fillOrderWatcher   *eventemitter.Watcher
	cancelOrderWatcher *eventemitter.Watcher
	cutoffAllWatcher   *eventemitter.Watcher
	cutoffPairWatcher  *eventemitter.Watcher
	forkWatcher        *eventemitter.Watcher
}

func NewOrderManager(
	options *config.OrderManagerOptions,
	rds dao.RdsService,
	userManager usermanager.UserManager,
	market marketcap.MarketCapProvider) *OrderManagerImpl {

	om := &OrderManagerImpl{}
	om.options = options
	om.rds = rds
	om.processor = NewForkProcess(om.rds, market)
	om.um = userManager
	om.mc = market
	om.cutoffCache = NewCutoffCache(options.CutoffCacheCleanTime)

	dustOrderValue = om.options.DustOrderValue

	return om
}

// Start start orderbook as a service
func (om *OrderManagerImpl) Start() {
	om.newOrderWatcher = &eventemitter.Watcher{Concurrent: false, Handle: om.handleGatewayOrder}
	om.ringMinedWatcher = &eventemitter.Watcher{Concurrent: false, Handle: om.handleRingMined}
	om.fillOrderWatcher = &eventemitter.Watcher{Concurrent: false, Handle: om.handleOrderFilled}
	om.cancelOrderWatcher = &eventemitter.Watcher{Concurrent: false, Handle: om.handleOrderCancelled}
	om.cutoffAllWatcher = &eventemitter.Watcher{Concurrent: false, Handle: om.handleCutoffAll}
	om.cutoffPairWatcher = &eventemitter.Watcher{Concurrent: false, Handle: om.handleCutoffPair}
	om.forkWatcher = &eventemitter.Watcher{Concurrent: false, Handle: om.handleFork}

	eventemitter.On(eventemitter.NewOrder, om.newOrderWatcher)
	eventemitter.On(eventemitter.RingMined, om.ringMinedWatcher)
	eventemitter.On(eventemitter.OrderFilledEvent, om.fillOrderWatcher)
	eventemitter.On(eventemitter.OrderCancelledEvent, om.cancelOrderWatcher)
	eventemitter.On(eventemitter.CutoffAllEvent, om.cutoffAllWatcher)
	eventemitter.On(eventemitter.CutoffPairEvent, om.cutoffPairWatcher)
	eventemitter.On(eventemitter.ChainForkProcess, om.forkWatcher)
}

func (om *OrderManagerImpl) Stop() {
	eventemitter.Un(eventemitter.NewOrder, om.newOrderWatcher)
	eventemitter.Un(eventemitter.RingMined, om.ringMinedWatcher)
	eventemitter.Un(eventemitter.OrderFilledEvent, om.fillOrderWatcher)
	eventemitter.Un(eventemitter.OrderCancelledEvent, om.cancelOrderWatcher)
	eventemitter.Un(eventemitter.CutoffAllEvent, om.cutoffAllWatcher)
	eventemitter.Un(eventemitter.CutoffPairEvent, om.cutoffPairWatcher)
	eventemitter.Un(eventemitter.ChainForkProcess, om.forkWatcher)
}

func (om *OrderManagerImpl) handleFork(input eventemitter.EventData) error {
	if err := om.processor.Fork(input.(*types.ForkedEvent)); err != nil {
		log.Fatalf("order manager,handle fork error:%s", err.Error())
	}
	return nil
}

// 所有来自gateway的订单都是新订单
func (om *OrderManagerImpl) handleGatewayOrder(input eventemitter.EventData) error {
	state := input.(*types.OrderState)
	log.Debugf("order manager,handle gateway order,order.hash:%s amountS:%s", state.RawOrder.Hash.Hex(), state.RawOrder.AmountS.String())

	model, err := newOrderEntity(state, om.mc, nil)
	if err != nil {
		return err
	}

	return om.rds.Add(model)
}

func (om *OrderManagerImpl) handleRingMined(input eventemitter.EventData) error {
	event := input.(*types.RingMinedEvent)

	var (
		model = &dao.RingMinedEvent{}
		err   error
	)

	model, err = om.rds.FindRingMinedByRingIndex(event.RingIndex.String())
	if err == nil {
		return fmt.Errorf("order manager,handle ringmined event,ring %s has already exist", event.Ringhash.Hex())
	}
	if err = model.ConvertDown(event); err != nil {
		return err
	}

	if err = om.rds.Add(model); err != nil {
		return fmt.Errorf("order manager,handle ringmined event,insert ring error:%s", err.Error())
	}

	return nil
}

func (om *OrderManagerImpl) handleOrderFilled(input eventemitter.EventData) error {
	event := input.(*types.OrderFilledEvent)

	// save fill event
	_, err := om.rds.FindFillEventByRinghashAndOrderhash(event.Ringhash, event.OrderHash)
	if err == nil {
		log.Debugf("order manager,handle order filled event,fill already exist ringIndex:%s orderHash:%s", event.RingIndex.String(), event.OrderHash.Hex())
		return nil
	}

	newFillModel := &dao.FillEvent{}
	newFillModel.ConvertDown(event)
	newFillModel.Fork = false
	if err := om.rds.Add(newFillModel); err != nil {
		log.Debugf("order manager,handle order filled event error:order %s insert faild", event.OrderHash.Hex())
		return err
	}

	// get rds.Order and types.OrderState
	state := &types.OrderState{UpdatedBlock: event.BlockNumber}
	model, err := om.rds.GetOrderByHash(event.OrderHash)
	if err != nil {
		return err
	}
	if err := model.ConvertUp(state); err != nil {
		return err
	}

	// judge order status
	if state.Status == types.ORDER_CUTOFF || state.Status == types.ORDER_FINISHED || state.Status == types.ORDER_UNKNOWN {
		log.Debugf("order manager,handle order filled event,order %s status is %d ", state.RawOrder.Hash.Hex(), state.Status)
		return nil
	}

	// calculate dealt amount
	state.UpdatedBlock = event.BlockNumber
	state.DealtAmountS = new(big.Int).Add(state.DealtAmountS, event.AmountS)
	state.DealtAmountB = new(big.Int).Add(state.DealtAmountB, event.AmountB)
	state.SplitAmountS = new(big.Int).Add(state.SplitAmountS, event.SplitS)
	state.SplitAmountB = new(big.Int).Add(state.SplitAmountB, event.SplitB)

	log.Debugf("order manager,handle order filled event orderhash:%s,dealAmountS:%s,dealtAmountB:%s", state.RawOrder.Hash.Hex(), state.DealtAmountS.String(), state.DealtAmountB.String())

	// update order status
	settleOrderStatus(state, om.mc)

	// update rds.Order
	if err := model.ConvertDown(state); err != nil {
		log.Errorf(err.Error())
		return err
	}
	if err := om.rds.UpdateOrderWhileFill(state.RawOrder.Hash, state.Status, state.DealtAmountS, state.DealtAmountB, state.SplitAmountS, state.SplitAmountB, state.UpdatedBlock); err != nil {
		return err
	}

	return nil
}

func (om *OrderManagerImpl) handleOrderCancelled(input eventemitter.EventData) error {
	event := input.(*types.OrderCancelledEvent)

	// save cancel event
	_, err := om.rds.GetCancelEvent(event.OrderHash, event.TxHash)
	if err == nil {
		log.Debugf("order manager,handle order cancelled event error:event %s have already exist", event.OrderHash.Hex())
		return nil
	}
	newCancelEventModel := &dao.CancelEvent{}
	newCancelEventModel.ConvertDown(event)
	newCancelEventModel.Fork = false
	if err := om.rds.Add(newCancelEventModel); err != nil {
		return err
	}

	// get rds.Order and types.OrderState
	state := &types.OrderState{}
	model, err := om.rds.GetOrderByHash(event.OrderHash)
	if err != nil {
		return err
	}
	if err := model.ConvertUp(state); err != nil {
		return err
	}

	// calculate remainAmount and cancelled amount should be saved whether order is finished or not
	if state.RawOrder.BuyNoMoreThanAmountB {
		state.CancelledAmountB = new(big.Int).Add(state.CancelledAmountB, event.AmountCancelled)
		log.Debugf("order manager,handle order cancelled event,order:%s cancelled amountb:%s", state.RawOrder.Hash.Hex(), state.CancelledAmountB.String())
	} else {
		state.CancelledAmountS = new(big.Int).Add(state.CancelledAmountS, event.AmountCancelled)
		log.Debugf("order manager,handle order cancelled event,order:%s cancelled amounts:%s", state.RawOrder.Hash.Hex(), state.CancelledAmountS.String())
	}

	// update order status
	settleOrderStatus(state, om.mc)
	state.UpdatedBlock = event.BlockNumber

	// update rds.Order
	if err := model.ConvertDown(state); err != nil {
		return err
	}
	if err := om.rds.UpdateOrderWhileCancel(state.RawOrder.Hash, state.Status, state.CancelledAmountS, state.CancelledAmountB, state.UpdatedBlock); err != nil {
		return err
	}

	return nil
}

// 所有cutoff event都应该存起来,但不是所有event都会影响订单
func (om *OrderManagerImpl) handleCutoffAll(input eventemitter.EventData) error {
	evt := input.(*types.CutoffEvent)

	// check tx exist
	_, err := om.rds.GetCutoffEvent(evt.TxHash)
	if err == nil {
		log.Debugf("order manager,handle order cutoff event error:event %s have already exist", evt.TxHash.Hex())
		return nil
	}

	lastCutoff := om.cutoffCache.GetCutoff(evt.Protocol, evt.Owner)

	var orderHashList []common.Hash

	// 首次存储到缓存，lastCutoff == currentCutoff
	if evt.CutoffTime.Cmp(lastCutoff) < 0 {
		log.Debugf("order manager,handle cutoff event, protocol:%s - owner:%s lastCutofftime:%s > currentCutoffTime:%s", evt.Protocol.Hex(), evt.Owner.Hex(), lastCutoff.String(), evt.CutoffTime.String())
	} else {
		om.cutoffCache.UpdateCutoff(evt.Protocol, evt.Owner, evt.CutoffTime)
		if orders, _ := om.rds.GetCutoffOrders(evt.Owner, evt.CutoffTime); len(orders) > 0 {
			for _, v := range orders {
				var state types.OrderState
				v.ConvertUp(&state)
				orderHashList = append(orderHashList, state.RawOrder.Hash)
			}
			om.rds.SetCutOffOrders(orderHashList, evt.BlockNumber)
		}
		log.Debugf("order manager,handle cutoff event, owner:%s, cutoffTimestamp:%s", evt.Owner.Hex(), evt.CutoffTime.String())
	}

	// save cutoff event
	evt.OrderHashList = orderHashList
	newCutoffEventModel := &dao.CutOffEvent{}
	newCutoffEventModel.ConvertDown(evt)
	newCutoffEventModel.Fork = false

	return om.rds.Add(newCutoffEventModel)
}

func (om *OrderManagerImpl) handleCutoffPair(input eventemitter.EventData) error {
	evt := input.(*types.CutoffPairEvent)

	// check tx exist
	_, err := om.rds.GetCutoffPairEvent(evt.TxHash)
	if err == nil {
		log.Debugf("order manager,handle order cutoffPair event error:event %s have already exist", evt.TxHash.Hex())
		return nil
	}

	lastCutoffPair := om.cutoffCache.GetCutoffPair(evt.Protocol, evt.Owner, evt.Token1, evt.Token2)

	var orderHashList []common.Hash
	// 首次存储到缓存，lastCutoffPair == currentCutoffPair
	if evt.CutoffTime.Cmp(lastCutoffPair) < 0 {
		log.Debugf("order manager,handle cutoffPair event, protocol:%s - owner:%s lastCutoffPairtime:%s > currentCutoffPairTime:%s", evt.Protocol.Hex(), evt.Owner.Hex(), lastCutoffPair.String(), evt.CutoffTime.String())
	} else {
		om.cutoffCache.UpdateCutoffPair(evt.Protocol, evt.Owner, evt.Token1, evt.Token2, evt.CutoffTime)
		if orders, _ := om.rds.GetCutoffPairOrders(evt.Owner, evt.Token1, evt.Token2, evt.CutoffTime); len(orders) > 0 {
			for _, v := range orders {
				var state types.OrderState
				v.ConvertUp(&state)
				orderHashList = append(orderHashList, state.RawOrder.Hash)
			}
			om.rds.SetCutOffOrders(orderHashList, evt.BlockNumber)
		}
		log.Debugf("order manager,handle cutoffPair event, owner:%s, token1:%s, token2:%s, cutoffTimestamp:%s", evt.Owner.Hex(), evt.Token1.Hex(), evt.Token2.Hex(), evt.CutoffTime.String())
	}

	// save transaction
	evt.OrderHashList = orderHashList
	newCutoffPairEventModel := &dao.CutOffPairEvent{}
	newCutoffPairEventModel.ConvertDown(evt)
	newCutoffPairEventModel.Fork = false

	return om.rds.Add(newCutoffPairEventModel)
}

func (om *OrderManagerImpl) IsOrderFullFinished(state *types.OrderState) bool {
	return isOrderFullFinished(state, om.mc)
}

func (om *OrderManagerImpl) IsValueDusted(tokenAddress common.Address, value *big.Rat) bool {
	if legalValue, err := om.mc.LegalCurrencyValue(tokenAddress, value); nil != err {
		return false
	} else {
		return isValueDusted(legalValue)
	}
}

func (om *OrderManagerImpl) MinerOrders(protocol, tokenS, tokenB common.Address, length int, startBlockNumber, endBlockNumber int64, filterOrderHashLists ...*types.OrderDelayList) []*types.OrderState {
	var (
		list         []*types.OrderState
		modelList    []*dao.Order
		err          error
		filterStatus = []types.OrderStatus{types.ORDER_FINISHED, types.ORDER_CUTOFF, types.ORDER_CANCEL}
	)

	for _, orderDelay := range filterOrderHashLists {
		orderHashes := []string{}
		for _, hash := range orderDelay.OrderHash {
			orderHashes = append(orderHashes, hash.Hex())
		}
		if len(orderHashes) > 0 && orderDelay.DelayedCount != 0 {
			if err = om.rds.MarkMinerOrders(orderHashes, orderDelay.DelayedCount); err != nil {
				log.Debugf("order manager,provide orders for miner error:%s", err.Error())
			}
		}
	}

	// 从数据库获取订单
	if modelList, err = om.rds.GetOrdersForMiner(protocol.Hex(), tokenS.Hex(), tokenB.Hex(), length, filterStatus, startBlockNumber, endBlockNumber); err != nil {
		return list
	}

	for _, v := range modelList {
		state := &types.OrderState{}
		v.ConvertUp(state)
		if om.um.InWhiteList(state.RawOrder.Owner) {
			list = append(list, state)
		} else {
			log.Debugf("order manager,owner:%s not in white list", state.RawOrder.Owner.Hex())
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
		om.SetOrderCancelled(&state)
		list = append(list, state)
	}

	return list, nil
}

func (om *OrderManagerImpl) GetOrders(query map[string]interface{}, statusList []types.OrderStatus, pageIndex, pageSize int) (dao.PageResult, error) {
	var (
		pageRes dao.PageResult
	)
	sL := make([]int, 0)
	for _, s := range statusList {
		sL = append(sL, int(s))
	}
	tmp, err := om.rds.OrderPageQuery(query, sL, pageIndex, pageSize)

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
			log.Debug("convertUp error occurs " + err.Error())
			continue
		}
		om.SetOrderCancelled(&state)
		pageRes.Data = append(pageRes.Data, state)
	}
	return pageRes, nil
}

func (om *OrderManagerImpl) GetOrderByHash(hash common.Hash) (orderState *types.OrderState, err error) {
	var result types.OrderState
	order, err := om.rds.GetOrderByHash(hash)
	if err != nil {
		return nil, err
	}

	if err := order.ConvertUp(&result); err != nil {
		return nil, err
	}

	om.SetOrderCancelled(&result)

	return &result, nil
}

func (om *OrderManagerImpl) UpdateBroadcastTimeByHash(hash common.Hash, bt int) error {
	return om.rds.UpdateBroadcastTimeByHash(hash.Hex(), bt)
}

func (om *OrderManagerImpl) FillsPageQuery(query map[string]interface{}, pageIndex, pageSize int) (result dao.PageResult, err error) {
	return om.rds.FillsPageQuery(query, pageIndex, pageSize)
}

func (om *OrderManagerImpl) RingMinedPageQuery(query map[string]interface{}, pageIndex, pageSize int) (result dao.PageResult, err error) {
	return om.rds.RingMinedPageQuery(query, pageIndex, pageSize)
}

func (om *OrderManagerImpl) IsOrderCutoff(protocol, owner, token1, token2 common.Address, validsince *big.Int) bool {
	return om.cutoffCache.IsOrderCutoff(protocol, owner, token1, token2, validsince)
}

func (om *OrderManagerImpl) SetOrderCancelled(state *types.OrderState) {
	if isOrderCancelled(state, om.mc) {
		state.Status = types.ORDER_CANCEL
	}
}

func (om *OrderManagerImpl) GetFrozenAmount(owner common.Address, token common.Address, statusSet []types.OrderStatus) (*big.Int, error) {
	orderList, err := om.rds.GetFrozenAmount(owner, token, statusSet)
	if err != nil {
		return nil, err
	}

	totalAmount := big.NewInt(0)

	if len(orderList) == 0 {
		return totalAmount, nil
	}

	for _, v := range orderList {
		var state types.OrderState
		if err := v.ConvertUp(&state); err != nil {
			continue
		}
		rs, _ := state.RemainedAmount()
		totalAmount.Add(totalAmount, rs.Num())
	}

	return totalAmount, nil
}

func (om *OrderManagerImpl) GetFrozenLRCFee(owner common.Address, statusSet []types.OrderStatus) (*big.Int, error) {
	orderList, err := om.rds.GetFrozenLrcFee(owner, statusSet)
	if err != nil {
		return nil, err
	}

	totalAmount := big.NewInt(0)

	if len(orderList) == 0 {
		return totalAmount, nil
	}

	for _, v := range orderList {
		lrcFee, _ := new(big.Int).SetString(v.LrcFee, 0)
		totalAmount.Add(totalAmount, lrcFee)
	}

	return totalAmount, nil
}
