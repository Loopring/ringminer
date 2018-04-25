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
	"github.com/Loopring/relay/dao"
	"github.com/Loopring/relay/eventemiter"
	"github.com/Loopring/relay/log"
	"github.com/Loopring/relay/market"
	"github.com/Loopring/relay/market/util"
	"github.com/Loopring/relay/types"
	"math/big"
)

type TransactionManager struct {
	db                         dao.RdsService
	accountmanager             *market.AccountManager
	approveEventWatcher        *eventemitter.Watcher
	orderCancelledEventWatcher *eventemitter.Watcher
	cutoffAllEventWatcher      *eventemitter.Watcher
	cutoffPairEventWatcher     *eventemitter.Watcher
	wethDepositEventWatcher    *eventemitter.Watcher
	wethWithdrawalEventWatcher *eventemitter.Watcher
	transferEventWatcher       *eventemitter.Watcher
	ethTransferEventWatcher    *eventemitter.Watcher
	forkDetectedEventWatcher   *eventemitter.Watcher
}

func NewTxManager(db dao.RdsService, accountmanager *market.AccountManager) TransactionManager {
	var tm TransactionManager
	tm.db = db
	tm.accountmanager = accountmanager

	return tm
}

// Start start orderbook as a service
func (tm *TransactionManager) Start() {
	tm.approveEventWatcher = &eventemitter.Watcher{Concurrent: false, Handle: tm.SaveApproveEvent}
	eventemitter.On(eventemitter.Approve, tm.approveEventWatcher)

	tm.orderCancelledEventWatcher = &eventemitter.Watcher{Concurrent: false, Handle: tm.SaveOrderCancelledEvent}
	eventemitter.On(eventemitter.CancelOrder, tm.orderCancelledEventWatcher)

	tm.cutoffAllEventWatcher = &eventemitter.Watcher{Concurrent: false, Handle: tm.SaveCutoffAllEvent}
	eventemitter.On(eventemitter.CutoffAll, tm.cutoffAllEventWatcher)

	tm.cutoffPairEventWatcher = &eventemitter.Watcher{Concurrent: false, Handle: tm.SaveCutoffPairEvent}
	eventemitter.On(eventemitter.CutoffPair, tm.cutoffPairEventWatcher)

	tm.wethDepositEventWatcher = &eventemitter.Watcher{Concurrent: false, Handle: tm.SaveWethDepositEvent}
	eventemitter.On(eventemitter.WethDeposit, tm.wethDepositEventWatcher)

	tm.wethWithdrawalEventWatcher = &eventemitter.Watcher{Concurrent: false, Handle: tm.SaveWethWithdrawalEvent}
	eventemitter.On(eventemitter.WethWithdrawal, tm.wethWithdrawalEventWatcher)

	tm.transferEventWatcher = &eventemitter.Watcher{Concurrent: false, Handle: tm.SaveTransferEvent}
	eventemitter.On(eventemitter.Transfer, tm.transferEventWatcher)

	tm.ethTransferEventWatcher = &eventemitter.Watcher{Concurrent: false, Handle: tm.SaveEthTransferEvent}
	eventemitter.On(eventemitter.EthTransferEvent, tm.ethTransferEventWatcher)

	tm.forkDetectedEventWatcher = &eventemitter.Watcher{Concurrent: false, Handle: tm.ForkProcess}
	eventemitter.On(eventemitter.ChainForkDetected, tm.forkDetectedEventWatcher)
}

func (tm *TransactionManager) Stop() {
	eventemitter.Un(eventemitter.Approve, tm.approveEventWatcher)
	eventemitter.Un(eventemitter.CancelOrder, tm.orderCancelledEventWatcher)
	eventemitter.Un(eventemitter.CutoffAll, tm.cutoffAllEventWatcher)
	eventemitter.Un(eventemitter.CutoffPair, tm.cutoffPairEventWatcher)
	eventemitter.Un(eventemitter.WethDeposit, tm.wethDepositEventWatcher)
	eventemitter.Un(eventemitter.WethWithdrawal, tm.wethWithdrawalEventWatcher)
	eventemitter.Un(eventemitter.Transfer, tm.transferEventWatcher)
	eventemitter.Un(eventemitter.EthTransferEvent, tm.ethTransferEventWatcher)
	eventemitter.Un(eventemitter.ChainForkDetected, tm.forkDetectedEventWatcher)
}

func (tm *TransactionManager) ForkProcess(input eventemitter.EventData) error {
	log.Debugf("txmanager,processing chain fork......")

	tm.Stop()
	forkEvent := input.(*types.ForkedEvent)
	from := forkEvent.ForkBlock.Int64()
	to := forkEvent.DetectedBlock.Int64()
	if err := tm.db.RollBackTransaction(from, to); err != nil {
		log.Fatalf("txmanager,process fork error:%s", err.Error())
	}
	tm.Start()

	return nil
}

func (tm *TransactionManager) SaveApproveEvent(input eventemitter.EventData) error {
	evt := input.(*types.ApprovalEvent)

	var (
		tx  types.Transaction
		err error
	)
	tx.FromApproveEvent(evt)
	if tx.Symbol, err = util.GetSymbolWithAddress(tx.Protocol); err != nil {
		return err
	}
	return tm.saveTransaction(&tx)
}

func (tm *TransactionManager) SaveOrderCancelledEvent(input eventemitter.EventData) error {
	evt := input.(*types.OrderCancelledEvent)

	var tx types.Transaction
	tx.FromCancelEvent(evt)
	tx.Symbol = ETH_SYMBOL
	return tm.saveTransaction(&tx)
}

func (tm *TransactionManager) SaveCutoffAllEvent(input eventemitter.EventData) error {
	evt := input.(*types.CutoffEvent)

	var tx types.Transaction
	tx.FromCutoffEvent(evt)
	tx.Symbol = ETH_SYMBOL
	return tm.saveTransaction(&tx)
}

func (tm *TransactionManager) SaveCutoffPairEvent(input eventemitter.EventData) error {
	evt := input.(*types.CutoffPairEvent)

	var tx types.Transaction
	tx.FromCutoffPairEvent(evt)
	tx.Symbol = ETH_SYMBOL
	return tm.saveTransaction(&tx)
}

func (tm *TransactionManager) SaveWethDepositEvent(input eventemitter.EventData) error {
	evt := input.(*types.WethDepositEvent)

	var (
		tx  types.Transaction
		err error
	)

	// save weth
	tx.FromWethDepositEvent(evt)
	if tx.Symbol, err = util.GetSymbolWithAddress(tx.Protocol); err != nil {
		return err
	}
	if err := tm.saveTransaction(&tx); err != nil {
		return err
	}

	// save eth
	tx1 := tx
	tx1.Symbol = ETH_SYMBOL
	return tm.saveTransaction(&tx1)
}

func (tm *TransactionManager) SaveWethWithdrawalEvent(input eventemitter.EventData) error {
	evt := input.(*types.WethWithdrawalEvent)

	var (
		tx  types.Transaction
		err error
	)
	tx.FromWethWithdrawalEvent(evt)
	if tx.Symbol, err = util.GetSymbolWithAddress(tx.Protocol); err != nil {
		return nil
	}
	if err := tm.saveTransaction(&tx); err != nil {
		return err
	}

	// save eth
	tx1 := tx
	tx1.Symbol = ETH_SYMBOL
	return tm.saveTransaction(&tx1)
}

func (tm *TransactionManager) SaveTransferEvent(input eventemitter.EventData) error {
	evt := input.(*types.TransferEvent)

	var (
		tx  types.Transaction
		err error
	)
	tx.FromTransferEvent(evt)
	if tx.Symbol, err = util.GetSymbolWithAddress(tx.Protocol); err != nil {
		return nil
	}
	return tm.saveTransaction(&tx)
}

func (tm *TransactionManager) SaveOrderFilledEvent(input eventemitter.EventData) error {
	evt := input.(*types.OrderFilledEvent)

	var tx1, tx2 types.Transaction
	tx1.FromFillEvent(evt, types.TX_TYPE_BUY)
	tx1.Symbol = ""
	tm.saveTransaction(&tx1)

	tx2.FromFillEvent(evt, types.TX_TYPE_SELL)
	tx1.Symbol = ""
	tm.saveTransaction(&tx2)

	return nil
}

// 普通的transaction
// 当value大于0时认为是eth转账
// 当value等于0时认为是调用系统不支持的合约,默认使用fromTransferEvent/send type为unsupported_contract
func (tm *TransactionManager) SaveEthTransferEvent(input eventemitter.EventData) error {
	evt := input.(*types.TransferEvent)

	if evt.Amount.Cmp(big.NewInt(0)) > 0 {
		var tx types.Transaction

		tx.FromTransferEvent(evt)
		tx.Protocol = types.NilAddress
		tx.Symbol = ETH_SYMBOL
		if err := tm.saveTransaction(&tx); err != nil {
			return err
		}
	} else {
		var tx types.Transaction
		tx.FromTransferEvent(evt)
		tx.Type = types.TX_TYPE_UNSUPPORTED_CONTRACT
		tx.Protocol = tx.To
		tx.Symbol = ETH_SYMBOL
		if err := tm.saveTransaction(&tx); err != nil {
			return err
		}
	}

	return nil
}

func (tm *TransactionManager) saveTransaction(tx *types.Transaction) error {
	// validate tx addresses
	if !tm.validateTransaction(tx) {
		return nil
	}

	// save pending
	if tx.Status == types.TX_STATUS_PENDING {
		return tm.savePendingTransactions(tx)
	} else {
		return tm.saveMinedTransactions(tx)
	}
}

func (tm *TransactionManager) savePendingTransactions(tx *types.Transaction) error {
	// find transaction which have the same hash, raw_from and nonce
	if _, err := tm.db.GetPendingTransaction(tx.TxHash, tx.RawFrom, tx.Nonce); err == nil {
		return nil
	}

	return tm.addTransaction(tx)
}

func (tm *TransactionManager) saveMinedTransactions(tx *types.Transaction) error {
	// get transactions by sender and tx.nonce
	list, _ := tm.db.GetTransactionsBySenderNonce(tx.RawFrom, tx.Nonce)

	// insert new data if list is empty
	if len(list) == 0 {
		return tm.addTransaction(tx)
	}

	// judge have any pending tx and the same tx
	hasPendingTx := false
	hasSameTx := false
	for _, v := range list {
		var latest types.Transaction
		v.ConvertUp(&latest)

		if latest.Status == types.TX_STATUS_PENDING {
			hasPendingTx = true
		} else if latest.Compare(tx) {
			hasSameTx = true
		}
	}

	// delete pending tx
	if hasPendingTx {
		tm.db.DeletePendingTransactions(tx.RawFrom, tx.Nonce)
	}
	// check tx if exist
	if !hasSameTx {
		tm.addTransaction(tx)
	}
	return nil
}

func (tm *TransactionManager) addTransaction(tx *types.Transaction) error {
	var (
		latest dao.Transaction
		err    error
	)

	latest.ConvertDown(tx)
	err = tm.db.Add(&latest)

	if err == nil {
		eventemitter.Emit(eventemitter.TransactionEvent, tx)
	}

	return err
}

func (tm *TransactionManager) validateTransaction(tx *types.Transaction) bool {
	var fromUnlocked, toUnlocked bool

	unlocked := true

	// validate wallet address
	fromUnlocked, _ = tm.accountmanager.HasUnlocked(tx.From.Hex())
	if tx.Type == types.TX_TYPE_TRANSFER {
		toUnlocked, _ = tm.accountmanager.HasUnlocked(tx.To.Hex())
		if !fromUnlocked && !toUnlocked {
			unlocked = false
		}
	} else {
		if !fromUnlocked {
			unlocked = false
		}
	}

	log.Debugf("txmanager,save transaction,tx:%s, type:%s, status:%s, rawFrom:%s, rawTo:%s, from:%s->unlocked:%t, to:%s->unlocked:%t",
		tx.TxHash.Hex(), tx.TypeStr(), tx.StatusStr(), tx.RawFrom.Hex(), tx.RawTo.Hex(), tx.From.Hex(), fromUnlocked, tx.To.Hex(), toUnlocked)

	return unlocked
}

//func (tm *TransactionManager) saveTransaction(tx *types.Transaction) error {
//	var model dao.Transaction
//
//	tx.CreateTime = tx.BlockTime
//	tx.UpdateTime = tx.UpdateTime
//
//	model.ConvertDown(tx)
//
//	if unlocked, _ := tm.accountmanager.HasUnlocked(tx.Owner.Hex()); unlocked {
//		log.Debugf("txmanager,save transaction,tx:%s, type:%s, status:%s, rawFrom:%s, rawTo:%s, from:%s, to:%s", tx.TxHash.Hex(), tx.TypeStr(), tx.StatusStr(), tx.RawFrom.Hex(), tx.RawTo.Hex(), tx.From.Hex(), tx.To.Hex())
//
//		// emit transaction before saving
//		eventemitter.Emit(eventemitter.TransactionEvent, tx)
//
//		return tm.db.SaveTransaction(&model)
//	}
//
//	return nil
//}