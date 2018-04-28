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

package txmanager_test

import (
	"github.com/Loopring/relay/test"
	"github.com/Loopring/relay/txmanager"
	"github.com/Loopring/relay/types"
	"testing"
)

func TestTransactionViewImpl_GetPendingTransactions(t *testing.T) {
	txmanager.NewTxView(test.Rds())

	owner := "0x43e85E2c882bbcE41C69740Eed4BfFFb45E3f9dd"
	list, err := txmanager.GetPendingTransactions(owner)
	if err != nil {
		t.Fatalf(err.Error())
	}

	for _, v := range list {
		t.Logf("tx:%s, from:%s, to:%s, type:%s, status:%s", v.TxHash.Hex(), v.From.Hex(), v.To.Hex(), v.Type, v.Status)
	}
}

func TestTransactionViewImpl_GetAllTransactionCount(t *testing.T) {
	txmanager.NewTxView(test.Rds())

	owner := "0x43e85E2c882bbcE41C69740Eed4BfFFb45E3f9dd"
	symbol := "foo"
	status := "failed"
	typ := "all"
	if number, err := txmanager.GetAllTransactionCount(owner, symbol, status, typ); err != nil {
		t.Fatalf(err.Error())
	} else {
		t.Logf("owner:%s have %d transactions in %s", owner, number, symbol)
	}
}

func TestTransactionViewImpl_GetAllTransactions(t *testing.T) {
	txmanager.NewTxView(test.Rds())

	owner := "0x43e85E2c882bbcE41C69740Eed4BfFFb45E3f9dd"
	symbol := "foo"
	status := "all"
	typ := "all"

	txs, err := txmanager.GetAllTransactions(owner, symbol, status, typ, 20, 0)
	if err != nil {
		t.Fatalf(err.Error())
	}
	for k, v := range txs {
		t.Logf("%d >>>>>> txhash:%s, symbol:%s, from:%s, to:%s, type:%s, status:%s", k, v.TxHash.Hex(), v.Symbol, v.From.Hex(), v.To.Hex(), v.Type, v.Status)
	}
}

func TestTransactionViewImpl_DeleteDuplicatePendingTx(t *testing.T) {
	db := test.Rds()
	query := make(map[string]interface{})
	query["status"] = types.TX_STATUS_PENDING

	list, err := db.PendingTransactions(query)
	if err != nil {
		t.Fatalf(err.Error())
	}

	for _, v := range list {
		t.Log("=======================================================")
		t.Logf("symbol:%s, hash:%s, nonce:%s, owner:%s, status:%d", v.Symbol, v.TxHash, v.Nonce, v.Owner, v.Status)

		minedQuery := make(map[string]interface{})
		minedQuery["tx_hash"] = v.TxHash
		minedList, err := db.PendingTransactions(minedQuery)
		if err != nil {
			t.Fatalf(err.Error())
		}
		if len(minedList) > 1 {
			for _, mv := range minedList {
				if mv.Status != uint8(types.TX_STATUS_PENDING) {
					t.Logf("symbol:%s, hash:%s, nonce:%s, owner:%s, status:%d", mv.Symbol, mv.TxHash, mv.Nonce, mv.Owner, mv.Status)
				} else {
					//db.Del(mv)
				}
			}
		}
	}
}