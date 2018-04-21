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

package txmanager

import (
	"fmt"
	"github.com/Loopring/relay/market/util"
	"github.com/Loopring/relay/types"
	"github.com/ethereum/go-ethereum/common"
	"math/big"
	"strings"
)

const (
	ETH_SYMBOL  = "ETH"
	WETH_SYMBOL = "WETH"
)

type TransactionJsonResult struct {
	Protocol    common.Address     `json:"protocol"`
	From        common.Address     `json:"from"`
	To          common.Address     `json:"to"`
	TxHash      common.Hash        `json:"txHash"`
	Symbol      string             `json:"symbol"`
	Content     TransactionContent `json:"content"`
	BlockNumber int64              `json:"blockNumber"`
	Value       string             `json:"value"`
	LogIndex    int64              `json:"logIndex"`
	Type        string             `json:"type"`
	Status      string             `json:"status"`
	CreateTime  int64              `json:"createTime"`
	UpdateTime  int64              `json:"updateTime"`
	Nonce       string             `json:"nonce"`
}

func (tx1 *TransactionJsonResult) addTransferValue(tx2 *TransactionJsonResult) {
	v1, _ := new(big.Int).SetString(tx1.Value, 0)
	v2, _ := new(big.Int).SetString(tx2.Value, 0)
	tx1.Value = new(big.Int).Add(v1, v2).String()
}

func (tx *TransactionJsonResult) IsTransfer() bool {
	if tx.Type == types.TypeStr(types.TX_TYPE_SEND) || tx.Type == types.TypeStr(types.TX_TYPE_RECEIVE) {
		return true
	}
	return false
}

type TransactionContent struct {
	Market    string `json:"market"`
	OrderHash string `json:"orderHash"`
}

// 过滤老版本重复数据
func filter(tx *types.Transaction, owner common.Address, symbol string) error {
	askSymbol := strings.ToUpper(symbol)
	answerSymbol := strings.ToUpper(tx.Symbol)

	switch tx.Type {
	case types.TX_TYPE_SEND:
		if tx.From != owner {
			return fmt.Errorf("transaction view:filter old version compeated send tx:%s, from:%s, to:%s, owner:%s", tx.TxHash.Hex(), tx.From.Hex(), tx.To.Hex(), owner.Hex())
		}

	case types.TX_TYPE_RECEIVE:
		if tx.To != owner {
			return fmt.Errorf("transaction view:filter old version compeated receive tx:%s, from:%s, to:%s, owner:%s", tx.TxHash.Hex(), tx.From.Hex(), tx.To.Hex(), owner.Hex())
		}

	case types.TX_TYPE_CONVERT_INCOME:
		if askSymbol == ETH_SYMBOL && askSymbol != answerSymbol {
			return fmt.Errorf("transaction view:filter old version compeated weth deposit tx:%s, ask symbol:%s, answer symbol:%s", tx.TxHash, askSymbol, answerSymbol)
		}
		if askSymbol == WETH_SYMBOL && askSymbol != answerSymbol {
			return fmt.Errorf("transaction view:filter old version compeated weth withdrawal tx:%s, ask symbol:%s, answer symbol:%s", tx.TxHash, askSymbol, answerSymbol)
		}

	case types.TX_TYPE_CONVERT_OUTCOME:
		if askSymbol == ETH_SYMBOL && askSymbol != answerSymbol {
			return fmt.Errorf("transaction view:filter old version compeated weth deposit tx:%s, ask symbol:%s, answer symbol:%s", tx.TxHash, askSymbol, answerSymbol)
		}
		if askSymbol == WETH_SYMBOL && askSymbol != answerSymbol {
			return fmt.Errorf("transaction view:filter old version compeated weth withdrawal tx:%s, ask symbol:%s, answer symbol:%s", tx.TxHash, askSymbol, answerSymbol)
		}
	}

	return nil
}

func (dst *TransactionJsonResult) fromTransaction(tx *types.Transaction, owner common.Address, symbol string) {
	symbol = strings.ToUpper(symbol)

	switch tx.Type {
	case types.TX_TYPE_TRANSFER:
		if tx.From == owner {
			tx.Type = types.TX_TYPE_SEND
		} else {
			tx.Type = types.TX_TYPE_RECEIVE
		}

	case types.TX_TYPE_DEPOSIT:
		if symbol == ETH_SYMBOL {
			tx.Type = types.TX_TYPE_CONVERT_OUTCOME
			tx.Protocol = types.NilAddress
		} else {
			tx.Type = types.TX_TYPE_CONVERT_INCOME
		}

	case types.TX_TYPE_WITHDRAWAL:
		if symbol == ETH_SYMBOL {
			tx.Type = types.TX_TYPE_CONVERT_INCOME
			tx.Protocol = types.NilAddress
		} else {
			tx.Type = types.TX_TYPE_CONVERT_OUTCOME
		}

	case types.TX_TYPE_CUTOFF_PAIR:
		if ctx, err := tx.GetCutoffPairContent(); err == nil {
			if mkt, err := util.WrapMarketByAddress(ctx.Token1.Hex(), ctx.Token2.Hex()); err == nil {
				dst.Content = TransactionContent{Market: mkt}
			}
		}

	case types.TX_TYPE_CANCEL_ORDER:
		if ctx, err := tx.GetCancelOrderHash(); err == nil {
			dst.Content = TransactionContent{OrderHash: ctx}
		}
	}

	dst.Protocol = tx.Protocol
	dst.From = tx.From
	dst.To = tx.To
	dst.TxHash = tx.TxHash
	dst.BlockNumber = tx.BlockNumber.Int64()
	dst.LogIndex = tx.LogIndex
	dst.Type = tx.TypeStr()
	dst.Status = tx.StatusStr()
	dst.CreateTime = tx.CreateTime
	dst.UpdateTime = tx.UpdateTime
	dst.Symbol = tx.Symbol
	dst.Nonce = tx.TxInfo.Nonce.String()

	// set value
	if tx.Value == nil {
		dst.Value = "0"
	} else {
		dst.Value = tx.Value.String()
	}
}

//type MultiTransferContent struct {
//	symbol string
//	send []TransactionJsonResult
//	receive []TransactionJsonResult
//}
//
//// 将同一个tx里的transfer事件按照symbol&from&to进行整合
//// 1.同一个logIndex进行过滤
//// 2.同一个tx 如果包含某个transfer 则将其他的transfer打包到content
//func collector(src []TransactionJsonResult, owner common.Address, askSymbol string) []TransactionJsonResult {
//	var (
//		list         []TransactionJsonResult
//		txCombineMap = make(map[common.Hash]map[string][]TransactionJsonResult)
//		askSymbol    = standardSymbol(askSymbol)
//	)
//
//	for _, current := range src {
//		if _, ok := txCombineMap[current.TxHash]; !ok {
//			txCombineMap[current.TxHash] = make(map[string][]TransactionJsonResult)
//		}
//		if _, ok := txCombineMap[current.TxHash][current.Symbol]; !ok {
//			txCombineMap[current.TxHash][current.Symbol] = make([]TransactionJsonResult, 0)
//		}
//		txCombineMap[current.TxHash][current.Symbol] = append(txCombineMap[current.TxHash][current.Symbol], current)
//	}
//
//	for _, symbolCombineMap := range txCombineMap {
//		for symbol, resArr := range symbolCombineMap {
//
//			var (
//				res TransactionJsonResult
//				send = make(map[common.Address]TransactionJsonResult)
//				recv = make(map[common.Address]TransactionJsonResult)
//			)
//
//			for _, tx := range resArr {
//				if tx.Type == types.TypeStr(types.TX_TYPE_SEND) && owner == tx.From{
//					if _, ok := send[tx.To]; !ok {
//						send[tx.To] = tx
//					} else {
//						send[tx.To].addTransferValue(&tx)
//					}
//				}
//				if tx.Type == types.TypeStr(types.TX_TYPE_RECEIVE) && owner == tx.To {
//					if _, ok := recv[tx.From]; !ok {
//						recv[tx.From] = tx
//					} else {
//						recv[tx.From].addTransferValue(&tx)
//					}
//				}
//			}
//
//			if symbol == askSymbol {
//				res =
//			}
//		}
//
//		list = append(list, all...)
//	}
//
//	return list
//}

func standardSymbol(symbol string) string {
	return strings.ToUpper(symbol)
}

func protocolToSymbol(address common.Address) string {
	if address == types.NilAddress {
		return ETH_SYMBOL
	}
	symbol := util.AddressToAlias(address.Hex())
	return symbol
}

func symbolToProtocol(symbol string) common.Address {
	symbol = standardSymbol(symbol)
	if symbol == ETH_SYMBOL {
		return types.NilAddress
	}
	return util.AliasToAddress(symbol)
}
