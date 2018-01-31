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

package types

import (
	"github.com/ethereum/go-ethereum/common"
	"math/big"
)

// send/receive/sell/buy/wrap/unwrap/cancelOrder/approve
const (
	TX_STATUS_PENDING = 0
	TX_STATUS_SUCCESS = 1
	TX_STATUS_FAILED  = 2

	TX_TYPE_APPROVE      = 1
	TX_TYPE_SEND         = 2
	TX_TYPE_RECEIVE      = 3
	TX_TYPE_SELL         = 4
	TX_TYPE_BUY          = 5
	TX_TYPE_WRAP         = 6 // WETH DEPOSIT
	TX_TYPE_UNWRAP       = 7 // WETH WITHDRAWAL
	TX_TYPE_CANCEL_ORDER = 8
	TX_TYPE_CUTOFF       = 9
)

type Transaction struct {
	From        common.Address
	To          common.Address
	Hash        common.Hash
	BlockNumber *big.Int
	Value       *big.Int
	Type        uint8
	Status      uint8
	CreateTime  int64
	UpdateTime  int64
}

func (tx *Transaction) StatusStr() string {
	var ret string
	switch tx.Status {
	case TX_STATUS_PENDING:
		ret = "pending"
	case TX_STATUS_SUCCESS:
		ret = "success"
	case TX_STATUS_FAILED:
		ret = "failed"
	}

	return ret
}

func (tx *Transaction) TypeStr() string {
	var ret string

	switch tx.Type {
	case TX_TYPE_APPROVE:
		ret = "approve"
	case TX_TYPE_SEND:
		ret = "send"
	case TX_TYPE_RECEIVE:
		ret = "receive"
	case TX_TYPE_SELL:
		ret = "sell"
	case TX_TYPE_BUY:
		ret = "buy"
	case TX_TYPE_WRAP:
		ret = "wrap"
	case TX_TYPE_UNWRAP:
		ret = "unwrap"
	case TX_TYPE_CANCEL_ORDER:
		ret = "cancel_order"
	case TX_TYPE_CUTOFF:
		ret = "cutoff"
	}

	return ret
}
