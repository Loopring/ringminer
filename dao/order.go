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

package dao

import (
	"errors"
	"github.com/Loopring/relay/types"
	"github.com/ethereum/go-ethereum/common"
	"math/big"
	"time"
)

// order amountS 上限1e30

type Order struct {
	ID                    int     `gorm:"column:id;primary_key;"`
	Protocol              string  `gorm:"column:protocol;type:varchar(42)"`
	Owner                 string  `gorm:"column:owner;type:varchar(42)"`
	OrderHash             string  `gorm:"column:order_hash;type:varchar(82);unique_index"`
	TokenS                string  `gorm:"column:token_s;type:varchar(42)"`
	TokenB                string  `gorm:"column:token_b;type:varchar(42)"`
	AmountS               []byte  `gorm:"column:amount_s;type:varchar(30)"`
	AmountB               []byte  `gorm:"column:amount_b;type:varchar(30)"`
	CreateTime            int64   `gorm:"column:create_time;type:bigint"`
	Ttl                   int64   `gorm:"column:ttl;type:bigint"`
	Salt                  int64   `gorm:"column:salt;type:bigint"`
	LrcFee                []byte  `gorm:"column:lrc_fee;type:varchar(30)"`
	BuyNoMoreThanAmountB  bool    `gorm:"column:buy_nomore_than_amountb"`
	MarginSplitPercentage uint8   `gorm:"column:margin_split_percentage;type:tinyint(4)"`
	V                     uint8   `gorm:"column:v;type:tinyint(4)"`
	R                     string  `gorm:"column:r;type:varchar(66)"`
	S                     string  `gorm:"column:s;type:varchar(66)"`
	Price                 float64 `gorm:"column:price;type:decimal(28,16);"`
	BlockNumber           int64   `gorm:"column:block_num;type:bigint"`
	RemainAmountS         []byte  `gorm:"column:remain_amount_s;type:varchar(30)"`
	RemainAmountB         []byte  `gorm:"column:remain_amount_b;type:varchar(30)"`
	Status                uint8   `gorm:"column:status;type:tinyint(4)"`
	MinerBlockMark        int64   `gorm:"column:miner_block_mark;type:bigint"`
	BroadcastTime		  int     `gorm:"column:broadcast_time;type:bigint"`
}

// convert types/orderState to dao/order
func (o *Order) ConvertDown(state *types.OrderState) error {
	src := state.RawOrder

	var err error
	o.Price, _ = src.Price.Float64()
	if o.Price > 1e12 || o.Price < 0.0000000000000001 {
		return errors.New("price is out of range")
	}
	if o.AmountB, err = src.AmountB.MarshalText(); err != nil {
		return err
	}
	if o.AmountS, err = src.AmountS.MarshalText(); err != nil {
		return err
	}
	if o.RemainAmountB, err = state.RemainedAmountB.MarshalText(); err != nil {
		return err
	}
	if o.RemainAmountS, err = state.RemainedAmountS.MarshalText(); err != nil {
		return err
	}
	if o.LrcFee, err = src.LrcFee.MarshalText(); err != nil {
		return err
	}

	o.Protocol = src.Protocol.Hex()
	o.Owner = src.Owner.Hex()
	o.OrderHash = src.Hash.Hex()
	o.TokenB = src.TokenB.Hex()
	o.TokenS = src.TokenS.Hex()
	o.CreateTime = src.Timestamp.Int64()
	o.Ttl = src.Ttl.Int64()
	o.Salt = src.Salt.Int64()
	o.BuyNoMoreThanAmountB = src.BuyNoMoreThanAmountB
	o.MarginSplitPercentage = src.MarginSplitPercentage
	if state.BlockNumber != nil {
		o.BlockNumber = state.BlockNumber.Int64()
	}
	o.Status = uint8(state.Status)
	o.V = src.V
	o.S = src.S.Hex()
	o.R = src.R.Hex()
	o.BroadcastTime = state.BroadcastTime

	return nil
}

// convert dao/order to types/orderState
func (o *Order) ConvertUp(state *types.OrderState) error {
	dst := state.RawOrder

	dst.AmountS = new(big.Int)
	if err := dst.AmountS.UnmarshalText(o.AmountS); err != nil {
		return err
	}
	dst.AmountB = new(big.Int)
	if err := dst.AmountB.UnmarshalText(o.AmountB); err != nil {
		return err
	}
	state.RemainedAmountS = new(big.Int)
	if err := state.RemainedAmountS.UnmarshalText(o.RemainAmountS); err != nil {
		return err
	}
	state.RemainedAmountB = new(big.Int)
	if err := state.RemainedAmountB.UnmarshalText(o.RemainAmountB); err != nil {
		return err
	}
	dst.LrcFee = new(big.Int)
	if err := dst.LrcFee.UnmarshalText(o.LrcFee); err != nil {
		return err
	}
	dst.GeneratePrice()

	dst.Protocol = common.HexToAddress(o.Protocol)
	dst.TokenS = common.HexToAddress(o.TokenS)
	dst.TokenB = common.HexToAddress(o.TokenB)
	dst.Timestamp = big.NewInt(o.CreateTime)
	dst.Ttl = big.NewInt(o.Ttl)
	dst.Salt = big.NewInt(o.Salt)
	dst.BuyNoMoreThanAmountB = o.BuyNoMoreThanAmountB
	dst.MarginSplitPercentage = o.MarginSplitPercentage
	state.BlockNumber = big.NewInt(o.BlockNumber)
	state.Status = types.OrderStatus(o.Status)
	state.BroadcastTime = o.BroadcastTime
	dst.V = o.V
	dst.S = types.HexToBytes32(o.S)
	dst.R = types.HexToBytes32(o.R)
	dst.Owner = common.HexToAddress(o.Owner)

	dst.Hash = common.HexToHash(o.OrderHash)
	if dst.Hash != dst.GenerateHash() {
		return errors.New("dao order convert down generate hash error")
	}

	return nil
}

func (s *RdsServiceImpl) GetOrderByHash(orderhash common.Hash) (*Order, error) {
	order := &Order{}
	err := s.db.Where("order_hash = ?", orderhash.Hex()).First(order).Error
	return order, err
}

func (s *RdsServiceImpl) MarkMinerOrders(filterOrderhashs []string, blockNumber int64) error {
	err := s.db.Where("order_hash in (?)", filterOrderhashs).Update("miner_block_mark = ?", blockNumber).Error

	return err
}

func (s *RdsServiceImpl) UnMarkMinerOrders(blockNumber int64) error {
	return s.db.Where("miner_block_mark < ?", blockNumber).Update("miner_block_mark = ?", 0).Error
}

func (s *RdsServiceImpl) GetOrdersForMiner(tokenS, tokenB string, filterStatus []uint8) ([]Order, error) {
	var (
		list []Order
		err  error
	)

	if len(filterStatus) < 1 {
		return list, errors.New("should filter cutoff and finished orders")
	}

	nowtime := time.Now().Unix()
	err = s.db.
		Where("token_s = ? and token_b = ? and create_time + ttl > ? and status not in (?) and miner_block_mark = ?", tokenS, tokenB, nowtime, filterStatus, 0).
		Order("price desc").Find(&list).Error

	return list, err
}

func (s *RdsServiceImpl) GetOrdersWithBlockNumberRange(from, to int64) ([]Order, error) {
	var (
		list []Order
		err  error
	)

	if from < to {
		return list, errors.New("dao/order GetOrdersWithBlockNumberRange invalid block number")
	}

	err = s.db.Where("block_num between ? and ?", from, to).Find(&list).Error

	return list, err
}

func (s *RdsServiceImpl) GetCutoffOrders(cutoffTime int64) ([]Order, error) {
	var (
		list []Order
		err  error
	)

	err = s.db.Where("create_time < ?", cutoffTime).Find(&list).Error

	return list, err
}

func (s *RdsServiceImpl) CheckOrderCutoff(orderhash string, cutoff int64) bool {
	model := Order{}
	err := s.db.Where("order_hash = ? and create_time < ?").Find(&model).Error
	if err != nil {
		return false
	}

	return true
}

func (s *RdsServiceImpl) SettleOrdersStatus(orderhashs []string, status types.OrderStatus) error {
	err := s.db.Where("order_hash in (?)", orderhashs).Update("status = ?", status.Value()).Error
	return err
}

func (s *RdsServiceImpl) GetOrderBook(protocol, tokenS, tokenB common.Address, length int) ([]Order, error) {
	var (
		list []Order
		err  error
	)

	err = s.db.Where("protocol = ? and token_s = ? and token_b = ?", protocol.Hex(), tokenS.Hex(), tokenB.Hex()).
		Order("price desc").Limit(length).Find(&list).Error

	return list, err
}

func (s *RdsServiceImpl) OrderPageQuery(query *Order, pageIndex, pageSize int) (PageResult, error) {
	var (
		orders []Order
		err    error
		data   = make([]interface{}, 0)
	)

	if pageIndex <= 0 {
		pageIndex = 1
	}

	if pageSize <= 0 {
		pageSize = 20
	}

	err = s.db.Where(&query).Offset((pageIndex - 1) * pageSize).Limit(pageSize).Find(&orders).Error
	for i, v := range orders {
		data[i] = v
	}

	pageResult := PageResult{data, pageIndex, pageSize, 0}
	return pageResult, err
}

func (s *RdsServiceImpl) UpdateBroadcastTimeByHash(hash string, bt int) error {
	return s.db.Where("order_hash = ?", hash).Update("broadcast_time", bt).Error
}