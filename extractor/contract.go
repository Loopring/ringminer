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

package extractor

import (
	"fmt"
	"github.com/Loopring/relay/dao"
	"github.com/Loopring/relay/ethaccessor"
	"github.com/Loopring/relay/eventemiter"
	"github.com/Loopring/relay/log"
	"github.com/Loopring/relay/market"
	"github.com/Loopring/relay/market/util"
	"github.com/Loopring/relay/types"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"math/big"
)

type EventData struct {
	types.TxInfo
	Event  interface{}
	CAbi   *abi.ABI
	Id     common.Hash
	Name   string
	Topics []string
}

func newEventData(event *abi.Event, cabi *abi.ABI) EventData {
	var c EventData

	c.Id = event.Id()
	c.Name = event.Name
	c.CAbi = cabi

	return c
}

func (event *EventData) FullFilled(evtLog *ethaccessor.Log, tx *ethaccessor.Transaction, receipt *ethaccessor.TransactionReceipt, blockTime *big.Int) {
	event.TxInfo = setTxInfo(tx, receipt, blockTime)
	event.Topics = evtLog.Topics
	event.Protocol = common.HexToAddress(evtLog.Address)
	event.LogIndex = evtLog.LogIndex.Int64() + 1
	event.Status = types.TX_STATUS_SUCCESS
}

type MethodData struct {
	types.TxInfo
	Method interface{}
	CAbi   *abi.ABI
	Id     string
	Name   string
	Value  *big.Int
	Input  string
}

func newMethodData(method *abi.Method, cabi *abi.ABI) MethodData {
	var c MethodData

	c.Id = common.ToHex(method.Id())
	c.Name = method.Name
	c.CAbi = cabi

	return c
}

func (method *MethodData) FullFilled(tx *ethaccessor.Transaction, receipt *ethaccessor.TransactionReceipt, blockTime *big.Int, status uint8) {
	method.TxInfo = setTxInfo(tx, receipt, blockTime)
	method.Value = tx.Value.BigInt()
	method.Input = tx.Input
	method.LogIndex = 0
	method.Status = status
}

func setTxInfo(tx *ethaccessor.Transaction, receipt *ethaccessor.TransactionReceipt, blockTime *big.Int) types.TxInfo {
	var txinfo types.TxInfo

	txinfo.BlockNumber = tx.BlockNumber.BigInt()
	txinfo.BlockTime = blockTime.Int64()
	txinfo.BlockHash = common.HexToHash(tx.BlockHash)
	txinfo.TxHash = common.HexToHash(tx.Hash)
	txinfo.Protocol = common.HexToAddress(tx.To)
	txinfo.From = common.HexToAddress(tx.From)
	txinfo.To = common.HexToAddress(tx.To)
	txinfo.GasLimit = tx.Gas.BigInt()
	if receipt != nil {
		txinfo.GasUsed = receipt.GasUsed.BigInt()
	} else {
		txinfo.GasUsed = big.NewInt(0)
	}
	txinfo.GasPrice = tx.GasPrice.BigInt()
	txinfo.Nonce = tx.Nonce.BigInt()
	txinfo.Symbol = ""

	return txinfo
}

func (m *MethodData) IsValid() error {
	if m.Status == types.TX_STATUS_FAILED {
		return fmt.Errorf("method %s transaction failed", m.Name)
	}
	return nil
}

const (
	RINGMINED_EVT_NAME           = "RingMined"
	CANCEL_EVT_NAME              = "OrderCancelled"
	CUTOFF_EVT_NAME              = "AllOrdersCancelled"
	CUTOFFPAIR_EVT_NAME          = "OrdersCancelled"
	TOKENREGISTERED_EVT_NAME     = "TokenRegistered"
	TOKENUNREGISTERED_EVT_NAME   = "TokenUnregistered"
	ADDRESSAUTHORIZED_EVT_NAME   = "AddressAuthorized"
	ADDRESSDEAUTHORIZED_EVT_NAME = "AddressDeauthorized"
	TRANSFER_EVT_NAME            = "Transfer"
	APPROVAL_EVT_NAME            = "Approval"

	SUBMITRING_METHOD_NAME      = "submitRing"
	CANCELORDER_METHOD_NAME     = "cancelOrder"
	CUTOFF_METHOD_NAME          = "cancelAllOrders"
	CUTOFFPAIR_METHOD_NAME      = "cancelAllOrdersByTradingPair"
	WETH_DEPOSIT_METHOD_NAME    = "deposit"
	WETH_WITHDRAWAL_METHOD_NAME = "withdraw"
	APPROVAL_METHOD_NAME        = "approve"
)

type AbiProcessor struct {
	events         map[common.Hash]EventData
	methods        map[string]MethodData
	protocols      map[common.Address]string
	delegates      map[common.Address]string
	accountmanager *market.AccountManager
	db             dao.RdsService
}

// 这里无需考虑版本问题，对解析来说，不接受版本升级带来数据结构变化的可能性
func newAbiProcessor(db dao.RdsService, accountmanager *market.AccountManager) *AbiProcessor {
	processor := &AbiProcessor{}

	processor.accountmanager = accountmanager
	processor.events = make(map[common.Hash]EventData)
	processor.methods = make(map[string]MethodData)
	processor.protocols = make(map[common.Address]string)
	processor.delegates = make(map[common.Address]string)
	processor.db = db

	processor.loadProtocolAddress()
	processor.loadErc20Contract()
	processor.loadWethContract()
	processor.loadProtocolContract()
	processor.loadTokenRegisterContract()
	processor.loadTokenTransferDelegateProtocol()

	return processor
}

// GetEvent get EventData with id hash
func (processor *AbiProcessor) GetEvent(id common.Hash) (EventData, bool) {
	var (
		event EventData
		ok    bool
	)
	event, ok = processor.events[id]
	return event, ok
}

// GetMethod get MethodData with method id
func (processor *AbiProcessor) GetMethod(id string) (MethodData, bool) {
	var (
		method MethodData
		ok     bool
	)
	method, ok = processor.methods[id]
	return method, ok
}

// HasContract judge protocol have ever been load
func (processor *AbiProcessor) HasContract(protocol common.Address) bool {
	_, ok := processor.protocols[protocol]
	return ok
}

// HasSpender check approve spender address have ever been load
func (processor *AbiProcessor) HasSpender(spender common.Address) bool {
	_, ok := processor.delegates[spender]
	return ok
}

func (processor *AbiProcessor) loadProtocolAddress() {
	for _, v := range util.AllTokens {
		processor.protocols[v.Protocol] = v.Symbol
		log.Infof("extractor,contract protocol %s->%s", v.Symbol, v.Protocol.Hex())
	}

	for _, v := range ethaccessor.ProtocolAddresses() {
		protocolSymbol := "loopring"
		delegateSymbol := "transfer_delegate"
		tokenRegisterSymbol := "token_register"

		processor.protocols[v.ContractAddress] = protocolSymbol
		processor.protocols[v.TokenRegistryAddress] = tokenRegisterSymbol
		processor.protocols[v.DelegateAddress] = delegateSymbol

		processor.delegates[v.DelegateAddress] = delegateSymbol

		log.Infof("extractor,contract protocol %s->%s", protocolSymbol, v.ContractAddress.Hex())
		log.Infof("extractor,contract protocol %s->%s", tokenRegisterSymbol, v.TokenRegistryAddress.Hex())
		log.Infof("extractor,contract protocol %s->%s", delegateSymbol, v.DelegateAddress.Hex())
	}
}

func (processor *AbiProcessor) loadProtocolContract() {
	for name, event := range ethaccessor.ProtocolImplAbi().Events {
		if name != RINGMINED_EVT_NAME && name != CANCEL_EVT_NAME && name != CUTOFF_EVT_NAME && name != CUTOFFPAIR_EVT_NAME {
			continue
		}

		watcher := &eventemitter.Watcher{}
		contract := newEventData(&event, ethaccessor.ProtocolImplAbi())

		switch contract.Name {
		case RINGMINED_EVT_NAME:
			contract.Event = &ethaccessor.RingMinedEvent{}
			watcher = &eventemitter.Watcher{Concurrent: false, Handle: processor.handleRingMinedEvent}
		case CANCEL_EVT_NAME:
			contract.Event = &ethaccessor.OrderCancelledEvent{}
			watcher = &eventemitter.Watcher{Concurrent: false, Handle: processor.handleOrderCancelledEvent}
		case CUTOFF_EVT_NAME:
			contract.Event = &ethaccessor.CutoffEvent{}
			watcher = &eventemitter.Watcher{Concurrent: false, Handle: processor.handleCutoffEvent}
		case CUTOFFPAIR_EVT_NAME:
			contract.Event = &ethaccessor.CutoffPairEvent{}
			watcher = &eventemitter.Watcher{Concurrent: false, Handle: processor.handleCutoffPairEvent}
		}

		eventemitter.On(contract.Id.Hex(), watcher)
		processor.events[contract.Id] = contract
		log.Infof("extractor,contract event name:%s -> key:%s", contract.Name, contract.Id.Hex())
	}

	for name, method := range ethaccessor.ProtocolImplAbi().Methods {
		if name != SUBMITRING_METHOD_NAME && name != CANCELORDER_METHOD_NAME && name != CUTOFF_METHOD_NAME && name != CUTOFFPAIR_METHOD_NAME {
			continue
		}

		contract := newMethodData(&method, ethaccessor.ProtocolImplAbi())
		watcher := &eventemitter.Watcher{}

		switch contract.Name {
		case SUBMITRING_METHOD_NAME:
			contract.Method = &ethaccessor.SubmitRingMethod{}
			watcher = &eventemitter.Watcher{Concurrent: false, Handle: processor.handleSubmitRingMethod}
		case CANCELORDER_METHOD_NAME:
			contract.Method = &ethaccessor.CancelOrderMethod{}
			watcher = &eventemitter.Watcher{Concurrent: false, Handle: processor.handleCancelOrderMethod}
		case CUTOFF_METHOD_NAME:
			contract.Method = &ethaccessor.CutoffMethod{}
			watcher = &eventemitter.Watcher{Concurrent: false, Handle: processor.handleCutoffMethod}
		case CUTOFFPAIR_METHOD_NAME:
			contract.Method = &ethaccessor.CutoffPairMethod{}
			watcher = &eventemitter.Watcher{Concurrent: false, Handle: processor.handleCutoffPairMethod}
		}

		eventemitter.On(contract.Id, watcher)
		processor.methods[contract.Id] = contract
		log.Infof("extractor,contract method name:%s -> key:%s", contract.Name, contract.Id)
	}
}

func (processor *AbiProcessor) loadErc20Contract() {
	for name, event := range ethaccessor.Erc20Abi().Events {
		if name != TRANSFER_EVT_NAME {
			continue
		}

		watcher := &eventemitter.Watcher{}
		contract := newEventData(&event, ethaccessor.Erc20Abi())

		switch contract.Name {
		case TRANSFER_EVT_NAME:
			contract.Event = &ethaccessor.TransferEvent{}
			watcher = &eventemitter.Watcher{Concurrent: false, Handle: processor.handleTransferEvent}
		case APPROVAL_EVT_NAME:
			contract.Event = &ethaccessor.ApprovalEvent{}
			watcher = &eventemitter.Watcher{Concurrent: false, Handle: processor.handleApprovalEvent}
		}

		eventemitter.On(contract.Id.Hex(), watcher)
		processor.events[contract.Id] = contract
		log.Infof("extractor,contract event name:%s -> key:%s", contract.Name, contract.Id.Hex())
	}

	for name, method := range ethaccessor.Erc20Abi().Methods {
		if name != APPROVAL_METHOD_NAME {
			continue
		}

		contract := newMethodData(&method, ethaccessor.Erc20Abi())
		contract.Method = &ethaccessor.ApproveMethod{}
		watcher := &eventemitter.Watcher{Concurrent: false, Handle: processor.handleApproveMethod}
		eventemitter.On(contract.Id, watcher)

		processor.methods[contract.Id] = contract

		log.Infof("extractor,contract method name:%s -> key:%s", contract.Name, contract.Id)
	}
}

func (processor *AbiProcessor) loadWethContract() {
	for name, method := range ethaccessor.WethAbi().Methods {
		if name != WETH_DEPOSIT_METHOD_NAME && name != WETH_WITHDRAWAL_METHOD_NAME {
			continue
		}

		watcher := &eventemitter.Watcher{}
		contract := newMethodData(&method, ethaccessor.WethAbi())

		switch contract.Name {
		case WETH_DEPOSIT_METHOD_NAME:
			// weth deposit without any inputs,use transaction.value as input
			watcher = &eventemitter.Watcher{Concurrent: false, Handle: processor.handleWethDepositMethod}
		case WETH_WITHDRAWAL_METHOD_NAME:
			contract.Method = &ethaccessor.WethWithdrawalMethod{}
			watcher = &eventemitter.Watcher{Concurrent: false, Handle: processor.handleWethWithdrawalMethod}
		}

		eventemitter.On(contract.Id, watcher)
		processor.methods[contract.Id] = contract
		log.Infof("extractor,contract method name:%s -> key:%s", contract.Name, contract.Id)
	}
}

func (processor *AbiProcessor) loadTokenRegisterContract() {
	for name, event := range ethaccessor.TokenRegistryAbi().Events {
		if name != TOKENREGISTERED_EVT_NAME && name != TOKENUNREGISTERED_EVT_NAME {
			continue
		}

		watcher := &eventemitter.Watcher{}
		contract := newEventData(&event, ethaccessor.TokenRegistryAbi())

		switch contract.Name {
		case TOKENREGISTERED_EVT_NAME:
			contract.Event = &ethaccessor.TokenRegisteredEvent{}
			watcher = &eventemitter.Watcher{Concurrent: false, Handle: processor.handleTokenRegisteredEvent}
		case TOKENUNREGISTERED_EVT_NAME:
			contract.Event = &ethaccessor.TokenUnRegisteredEvent{}
			watcher = &eventemitter.Watcher{Concurrent: false, Handle: processor.handleTokenUnRegisteredEvent}
		}

		eventemitter.On(contract.Id.Hex(), watcher)
		processor.events[contract.Id] = contract
		log.Infof("extractor,contract event name:%s -> key:%s", contract.Name, contract.Id.Hex())
	}
}

func (processor *AbiProcessor) loadTokenTransferDelegateProtocol() {
	for name, event := range ethaccessor.DelegateAbi().Events {
		if name != ADDRESSAUTHORIZED_EVT_NAME && name != ADDRESSDEAUTHORIZED_EVT_NAME {
			continue
		}

		watcher := &eventemitter.Watcher{}
		contract := newEventData(&event, ethaccessor.DelegateAbi())

		switch contract.Name {
		case ADDRESSAUTHORIZED_EVT_NAME:
			contract.Event = &ethaccessor.AddressAuthorizedEvent{}
			watcher = &eventemitter.Watcher{Concurrent: false, Handle: processor.handleAddressAuthorizedEvent}
		case ADDRESSDEAUTHORIZED_EVT_NAME:
			contract.Event = &ethaccessor.AddressDeAuthorizedEvent{}
			watcher = &eventemitter.Watcher{Concurrent: false, Handle: processor.handleAddressDeAuthorizedEvent}
		}

		eventemitter.On(contract.Id.Hex(), watcher)
		processor.events[contract.Id] = contract
		log.Infof("extractor,contract event name:%s -> key:%s", contract.Name, contract.Id.Hex())
	}
}

// 只需要解析submitRing,cancel，cutoff这些方法在event里，如果方法不成功也不用执行后续逻辑
func (processor *AbiProcessor) handleSubmitRingMethod(input eventemitter.EventData) error {
	contract := input.(MethodData)

	// emit to miner
	var evt types.SubmitRingMethodEvent
	evt.TxInfo = contract.TxInfo
	evt.UsedGas = contract.GasUsed
	evt.UsedGasPrice = contract.GasPrice
	evt.Err = contract.IsValid()

	log.Debugf("extractor,tx:%s submitRing method gas:%s, gasprice:%s", evt.TxHash.Hex(), evt.UsedGas.String(), evt.UsedGasPrice.String())

	eventemitter.Emit(eventemitter.Miner_SubmitRing_Method, &evt)

	//ring := contract.Method.(*ethaccessor.SubmitRingMethod)
	//ring.Protocol = evt.Protocol
	//
	//data := hexutil.MustDecode("0x" + contract.Input[10:])
	//if err := contract.CAbi.UnpackMethodInput(ring, contract.Name, data); err != nil {
	//	log.Errorf("extractor,tx:%s submitRing method, unpack error:%s", evt.TxHash.Hex(), err.Error())
	//	return nil
	//}

	//orderList, err := ring.ConvertDown()
	//if err != nil {
	//	log.Errorf("extractor,tx:%s submitRing method convert order data error:%s", evt.TxHash.Hex(), err.Error())
	//	return nil
	//}
	//
	//// save order
	//for _, v := range orderList {
	//	v.Protocol = common.HexToAddress(contract.Protocol)
	//	v.Hash = v.GenerateHash()
	//	log.Debugf("extractor,tx:%s submitRing method orderHash:%s,owner:%s,tokenS:%s,tokenB:%s,amountS:%s,amountB:%s", evt.TxHash.Hex(), v.Hash.Hex(), v.Owner.Hex(), v.TokenS.Hex(), v.TokenB.Hex(), v.AmountS.String(), v.AmountB.String())
	//	eventemitter.Emit(eventemitter.Gateway, v)
	//}
	//
	//// save transactions while submitRing failed，otherwise save transactions while process ringmined event
	//if evt.TxFailed {
	//	processor.saveOrderListAsTxs(evt.TxHash, orderList, &contract)
	//}

	return nil
}

//func (processor *AbiProcessor) saveOrderListAsTxs(txhash common.Hash, orderList []*types.Order, contract *MethodData) {
//	length := len(orderList)
//
//	log.Debugf("extractor,tx:%s saveOrderListAsTxs:length %d and tx isFailed:%t", txhash.Hex(), length, contract.IsFailed)
//
//	nowtime := time.Now().Unix()
//
//	for i := 0; i < length; i++ {
//		var (
//			tx              types.Transaction
//			model1, model2  dao.Transaction
//			sellto, buyfrom common.Address
//		)
//		ord := orderList[i]
//		if i == length-1 {
//			sellto = orderList[0].Owner
//		} else {
//			sellto = orderList[i+1].Owner
//		}
//		if i == 0 {
//			buyfrom = orderList[length-1].Owner
//		} else {
//			buyfrom = orderList[i-1].Owner
//		}
//
//		// todo(fuk):emit as event,saved by wallet/relay but not extractor
//		tx.FromOrder(ord, txhash, sellto, types.TX_TYPE_SELL, types.TX_STATUS_FAILED, contract.BlockNumber, nowtime)
//		model1.ConvertDown(&tx)
//		processor.db.SaveTransaction(&model1)
//
//		tx.FromOrder(ord, txhash, buyfrom, types.TX_TYPE_BUY, types.TX_STATUS_FAILED, contract.BlockNumber, nowtime)
//		model2.ConvertDown(&tx)
//		processor.db.SaveTransaction(&model2)
//	}
//}

func (processor *AbiProcessor) handleCancelOrderMethod(input eventemitter.EventData) error {
	contract := input.(MethodData)
	cancel := contract.Method.(*ethaccessor.CancelOrderMethod)

	data := hexutil.MustDecode("0x" + contract.Input[10:])
	if err := contract.CAbi.UnpackMethodInput(cancel, contract.Name, data); err != nil {
		log.Errorf("extractor,tx:%s cancelOrder method unpack error:%s", contract.TxHash.Hex(), err.Error())
		return nil
	}

	order, cancelAmount, _ := cancel.ConvertDown()
	txinfo := contract.TxInfo

	log.Debugf("extractor,tx:%s cancelOrder method order tokenS:%s,tokenB:%s,amountS:%s,amountB:%s", contract.TxHash.Hex(), order.TokenS.Hex(), order.TokenB.Hex(), order.AmountS.String(), order.AmountB.String())

	order.Protocol = contract.Protocol

	// 不再存储取消的订单
	//eventemitter.Emit(eventemitter.Gateway, order)

	// save transactions while cancel order failed,other save transactions while process cancelOrderEvent
	if contract.Status == types.TX_STATUS_FAILED || contract.Status == types.TX_STATUS_PENDING {
		processor.saveCancelOrderMethodAsTx(txinfo, cancelAmount)
	}

	return nil
}

func (processor *AbiProcessor) saveCancelOrderMethodAsTx(txinfo types.TxInfo, amount *big.Int) error {
	log.Debugf("extractor,tx:%s saveCancelOrderMethodAsTx status:%d", txinfo.TxHash.Hex(), txinfo.Status)
	var tx types.Transaction
	tx.FromCancelMethod(txinfo, amount)
	return processor.saveTransaction(&tx)
}

func (processor *AbiProcessor) handleCutoffMethod(input eventemitter.EventData) error {
	contract := input.(MethodData)
	contractMethod := contract.Method.(*ethaccessor.CutoffMethod)

	data := hexutil.MustDecode("0x" + contract.Input[10:])
	if err := contract.CAbi.UnpackMethodInput(&contractMethod.Cutoff, contract.Name, data); err != nil {
		log.Errorf("extractor,tx:%s cutoff method unpack error:%s", contract.TxHash.Hex(), err.Error())
		return nil
	}

	cutoff := contractMethod.ConvertDown()
	cutoff.TxInfo = contract.TxInfo
	cutoff.Owner = cutoff.From
	log.Debugf("extractor,tx:%s cutoff method owner:%s, cutoff:%d, status:%d", contract.TxHash.Hex(), cutoff.Owner.Hex(), cutoff.Value.Int64(), cutoff.Status)

	if cutoff.Status == types.TX_STATUS_PENDING || cutoff.Status == types.TX_STATUS_FAILED {
		processor.saveCutoffMethodAsTx(cutoff)
	}

	return nil
}

func (processor *AbiProcessor) saveCutoffMethodAsTx(evt *types.CutoffMethodEvent) error {
	var tx types.Transaction

	tx.FromCutoffMethodEvent(evt)
	return processor.saveTransaction(&tx)
}

func (processor *AbiProcessor) handleCutoffPairMethod(input eventemitter.EventData) error {
	contract := input.(MethodData)
	contractMethod := contract.Method.(*ethaccessor.CutoffPairMethod)

	data := hexutil.MustDecode("0x" + contract.Input[10:])
	if err := contract.CAbi.UnpackMethodInput(contractMethod, contract.Name, data); err != nil {
		log.Errorf("extractor,tx:%s cutoffpair method unpack error:%s", contract.TxHash.Hex(), err.Error())
		return nil
	}

	cutoffpair := contractMethod.ConvertDown()
	cutoffpair.TxInfo = contract.TxInfo
	cutoffpair.Owner = cutoffpair.From

	log.Debugf("extractor,tx:%s cutoffpair method owenr:%s, token1:%s, token2:%s, cutoff:%d", contract.TxHash.Hex(), cutoffpair.Owner.Hex(), cutoffpair.Token1.Hex(), cutoffpair.Token2.Hex(), cutoffpair.Value.Int64())

	if cutoffpair.Status == types.TX_STATUS_PENDING || cutoffpair.Status == types.TX_STATUS_FAILED {
		processor.saveCutoffPairMethodAsTx(cutoffpair)
	}

	return nil
}

func (processor *AbiProcessor) saveCutoffPairMethodAsTx(evt *types.CutoffPairMethodEvent) error {
	var tx types.Transaction
	tx.FromCutoffPairMethod(evt)
	return processor.saveTransaction(&tx)
}

func (processor *AbiProcessor) handleApproveMethod(input eventemitter.EventData) error {
	contractData := input.(MethodData)
	contractMethod := contractData.Method.(*ethaccessor.ApproveMethod)

	data := hexutil.MustDecode("0x" + contractData.Input[10:])
	if err := contractData.CAbi.UnpackMethodInput(contractMethod, contractData.Name, data); err != nil {
		log.Errorf("extractor,tx:%s approve method unpack error:%s", contractData.TxHash.Hex(), err.Error())
		return nil
	}

	approve := contractMethod.ConvertDown()
	approve.Owner = contractData.From
	approve.TxInfo = contractData.TxInfo

	log.Debugf("extractor,tx:%s approve method owner:%s, spender:%s, value:%s", contractData.TxHash.Hex(), approve.Owner.Hex(), approve.Spender.Hex(), approve.Value.String())

	if processor.HasSpender(approve.Spender) {
		eventemitter.Emit(eventemitter.ApproveMethod, approve)
	}

	processor.saveApproveMethodAsTx(approve)
	return nil
}

func (processor *AbiProcessor) saveApproveMethodAsTx(evt *types.ApproveMethodEvent) error {
	var tx types.Transaction

	log.Debugf("extractor:tx:%s saveApproveMethodAsTx, status:%d", evt.TxHash.Hex(), evt.Status)

	tx.FromApproveMethod(evt)
	tx.Symbol, _ = util.GetSymbolWithAddress(tx.Protocol)

	return processor.saveTransaction(&tx)
}

func (processor *AbiProcessor) handleWethDepositMethod(input eventemitter.EventData) error {
	contractData := input.(MethodData)

	var deposit types.WethDepositMethodEvent
	deposit.Owner = contractData.From
	deposit.Value = contractData.Value
	deposit.TxInfo = contractData.TxInfo

	log.Debugf("extractor,tx:%s wethDeposit method from:%s, to:%s, value:%s", contractData.TxHash.Hex(), deposit.From.Hex(), deposit.To.Hex(), deposit.Value.String())

	eventemitter.Emit(eventemitter.WethDepositMethod, &deposit)

	processor.saveWethDepositMethodAsTx(&deposit)
	return nil
}

func (processor *AbiProcessor) saveWethDepositMethodAsTx(evt *types.WethDepositMethodEvent) error {
	var tx types.Transaction

	log.Debugf("extractor:tx:%s saveWethDepositMethodAsTx", evt.TxHash.Hex())

	tx.FromWethDepositMethod(evt)
	tx.Symbol, _ = util.GetSymbolWithAddress(tx.Protocol)
	return processor.saveTransaction(&tx)
}

func (processor *AbiProcessor) handleWethWithdrawalMethod(input eventemitter.EventData) error {
	contractData := input.(MethodData)
	contractMethod := contractData.Method.(*ethaccessor.WethWithdrawalMethod)

	data := hexutil.MustDecode("0x" + contractData.Input[10:])
	if err := contractData.CAbi.UnpackMethodInput(&contractMethod.Value, contractData.Name, data); err != nil {
		log.Errorf("extractor,tx:%s wethWithdrawal method unpack error:%s", contractData.TxHash.Hex(), err.Error())
		return nil
	}

	withdrawal := contractMethod.ConvertDown()
	withdrawal.Owner = contractData.From
	withdrawal.TxInfo = contractData.TxInfo

	log.Debugf("extractor,tx:%s wethWithdrawal method from:%s, to:%s, value:%s", contractData.TxHash.Hex(), withdrawal.From.Hex(), withdrawal.To.Hex(), withdrawal.Value.String())

	eventemitter.Emit(eventemitter.WethWithdrawalMethod, withdrawal)

	processor.saveWethWithdrawalMethodAsTx(withdrawal)
	return nil
}

func (processor *AbiProcessor) saveWethWithdrawalMethodAsTx(evt *types.WethWithdrawalMethodEvent) error {
	var tx types.Transaction

	log.Debugf("extractor:tx:%s saveWethWithdrawalMethodAsTx", evt.TxHash.Hex())

	tx.FromWethWithdrawalMethod(evt)
	tx.Symbol, _ = util.GetSymbolWithAddress(tx.Protocol)
	return processor.saveTransaction(&tx)
}

func (processor *AbiProcessor) handleRingMinedEvent(input eventemitter.EventData) error {
	contractData := input.(EventData)
	if len(contractData.Topics) < 2 {
		log.Errorf("extractor,tx:%s ringMined event indexed fields number error", contractData.TxHash.Hex())
		return nil
	}

	contractEvent := contractData.Event.(*ethaccessor.RingMinedEvent)
	contractEvent.RingHash = common.HexToHash(contractData.Topics[1])

	ringmined, fills, err := contractEvent.ConvertDown()
	if err != nil {
		log.Errorf("extractor,tx:%s ringMined event convert down error:%s", contractData.TxHash.Hex(), err.Error())
		return nil
	}
	ringmined.TxInfo = contractData.TxInfo

	log.Debugf("extractor,tx:%s ringMined event ringhash:%s, ringIndex:%s, tx:%s",
		contractData.TxHash.Hex(),
		ringmined.Ringhash.Hex(),
		ringmined.RingIndex.String(),
		ringmined.TxHash.Hex())

	eventemitter.Emit(eventemitter.OrderManagerExtractorRingMined, ringmined)

	var (
		fillList, saveFillList []*types.OrderFilledEvent
		orderhashList          []string
	)
	for _, fill := range fills {
		fill.TxInfo = contractData.TxInfo

		log.Debugf("extractor,tx:%s orderFilled event ringhash:%s, amountS:%s, amountB:%s, orderhash:%s, lrcFee:%s, lrcReward:%s, nextOrderhash:%s, preOrderhash:%s, ringIndex:%s",
			contractData.TxHash.Hex(),
			fill.Ringhash.Hex(),
			fill.AmountS.String(),
			fill.AmountB.String(),
			fill.OrderHash.Hex(),
			fill.LrcFee.String(),
			fill.LrcReward.String(),
			fill.NextOrderHash.Hex(),
			fill.PreOrderHash.Hex(),
			fill.RingIndex.String(),
		)

		fillList = append(fillList, fill)
		orderhashList = append(orderhashList, fill.OrderHash.Hex())
	}

	ordermap, err := processor.db.GetOrdersByHash(orderhashList)
	if err != nil {
		log.Errorf("extractor,tx:%s ringMined event getOrdersByHash error:%s", contractData.TxHash.Hex(), err.Error())
		return nil
	}

	for _, v := range fillList {
		if ord, ok := ordermap[v.OrderHash.Hex()]; ok {
			v.TokenS = common.HexToAddress(ord.TokenS)
			v.TokenB = common.HexToAddress(ord.TokenB)
			v.Owner = common.HexToAddress(ord.Owner)
			v.Market, _ = util.WrapMarketByAddress(v.TokenB.Hex(), v.TokenS.Hex())
			eventemitter.Emit(eventemitter.OrderManagerExtractorFill, v)

			saveFillList = append(saveFillList, v)
		} else {
			log.Debugf("extractor,tx:%s orderFilled event cann't match order %s", contractData.TxHash.Hex(), ord.OrderHash)
		}
	}

	// 只存储跟订单相关的fill
	processor.saveFillListAsTxs(saveFillList, &contractData)
	return nil
}

func (processor *AbiProcessor) saveFillListAsTxs(fillList []*types.OrderFilledEvent, contract *EventData) {
	length := len(fillList)

	log.Debugf("extractor,tx:%s saveFillListAsTxs:length %d and tx status:%d", contract.TxHash.Hex(), length, contract.Status)

	for i := 0; i < length; i++ {
		var (
			tx1, tx2        types.Transaction
			sellto, buyfrom common.Address
		)
		fill := fillList[i]
		if i == length-1 {
			sellto = fillList[0].Owner
		} else {
			sellto = fillList[i+1].Owner
		}
		if i == 0 {
			buyfrom = fillList[length-1].Owner
		} else {
			buyfrom = fillList[i-1].Owner
		}

		tx1.FromFillEvent(fill, sellto, types.TX_TYPE_SELL)
		processor.saveTransaction(&tx1)

		tx2.FromFillEvent(fill, buyfrom, types.TX_TYPE_BUY)
		processor.saveTransaction(&tx2)
	}
}

func (processor *AbiProcessor) handleOrderCancelledEvent(input eventemitter.EventData) error {
	contractData := input.(EventData)
	if len(contractData.Topics) < 2 {
		log.Errorf("extractor,tx:%s orderCancelled event indexed fields number error", contractData.TxHash.Hex())
		return nil
	}

	contractEvent := contractData.Event.(*ethaccessor.OrderCancelledEvent)
	contractEvent.OrderHash = common.HexToHash(contractData.Topics[1])

	evt := contractEvent.ConvertDown()
	evt.TxInfo = contractData.TxInfo

	log.Debugf("extractor,tx:%s orderCancelled event orderhash:%s, cancelAmount:%s", contractData.TxHash.Hex(), evt.OrderHash.Hex(), evt.AmountCancelled.String())

	eventemitter.Emit(eventemitter.OrderManagerExtractorCancel, evt)

	processor.saveCancelOrderEventAsTx(evt)
	return nil
}

func (processor *AbiProcessor) saveCancelOrderEventAsTx(evt *types.OrderCancelledEvent) error {
	var tx types.Transaction
	tx.FromCancelEvent(evt, evt.From)
	return processor.saveTransaction(&tx)
}

func (processor *AbiProcessor) handleCutoffEvent(input eventemitter.EventData) error {
	contractData := input.(EventData)
	if len(contractData.Topics) < 2 {
		log.Errorf("extractor,tx:%s cutoffTimestampChanged event indexed fields number error", contractData.TxHash.Hex())
		return nil
	}

	contractEvent := contractData.Event.(*ethaccessor.CutoffEvent)
	contractEvent.Owner = common.HexToAddress(contractData.Topics[1])

	evt := contractEvent.ConvertDown()
	evt.TxInfo = contractData.TxInfo

	log.Debugf("extractor,tx:%s cutoffTimestampChanged event ownerAddress:%s, cutOffTime:%s, status:%d", contractData.TxHash.Hex(), evt.Owner.Hex(), evt.Cutoff.String(), evt.Status)

	eventemitter.Emit(eventemitter.OrderManagerExtractorCutoff, evt)

	processor.saveCutoffEventAsTx(evt)
	return nil
}

func (processor *AbiProcessor) saveCutoffEventAsTx(evt *types.CutoffEvent) error {
	var tx types.Transaction
	tx.FromCutoffEvent(evt)
	return processor.saveTransaction(&tx)
}

func (processor *AbiProcessor) handleCutoffPairEvent(input eventemitter.EventData) error {
	contractData := input.(EventData)
	if len(contractData.Topics) < 2 {
		log.Errorf("extractor,tx:%s cutoffPair event indexed fields number error", contractData.TxHash.Hex())
		return nil
	}

	contractEvent := contractData.Event.(*ethaccessor.CutoffPairEvent)
	contractEvent.Owner = common.HexToAddress(contractData.Topics[1])

	evt := contractEvent.ConvertDown()
	evt.TxInfo = contractData.TxInfo

	log.Debugf("extractor,tx:%s cutoffPair event ownerAddress:%s, token1:%s, token2:%s, cutOffTime:%s", contractData.TxHash.Hex(), evt.Owner.Hex(), evt.Token1.Hex(), evt.Token2.Hex(), evt.Cutoff.String())

	eventemitter.Emit(eventemitter.OrderManagerExtractorCutoffPair, evt)

	processor.saveCutoffPairEventAsTx(evt)

	return nil
}

func (processor *AbiProcessor) saveCutoffPairEventAsTx(evt *types.CutoffPairEvent) error {
	var tx types.Transaction
	tx.FromCutoffPairEvent(evt)
	return processor.saveTransaction(&tx)
}

func (processor *AbiProcessor) handleTransferEvent(input eventemitter.EventData) error {
	contractData := input.(EventData)

	if len(contractData.Topics) < 3 {
		log.Errorf("extractor,tx:%s tokenTransfer event indexed fields number error", contractData.TxHash.Hex())
		return nil
	}

	contractEvent := contractData.Event.(*ethaccessor.TransferEvent)
	contractEvent.From = common.HexToAddress(contractData.Topics[1])
	contractEvent.To = common.HexToAddress(contractData.Topics[2])

	transfer := contractEvent.ConvertDown()
	transfer.Sender = contractEvent.From
	transfer.Receiver = contractEvent.To
	transfer.TxInfo = contractData.TxInfo

	log.Debugf("extractor,tx:%s tokenTransfer event from:%s, to:%s, value:%s", contractData.TxHash.Hex(), transfer.From.Hex(), transfer.To.Hex(), transfer.Value.String())

	eventemitter.Emit(eventemitter.AccountTransfer, transfer)

	processor.saveTransferEventsAsTxs(transfer)
	return nil
}

func (processor *AbiProcessor) saveTransferEventsAsTxs(evt *types.TransferEvent) error {
	var tx1, tx2 types.Transaction

	log.Debugf("extractor:tx:%s saveTransferAsTx", evt.TxHash.Hex())

	tx1.FromTransferEvent(evt, types.TX_TYPE_SEND)
	tx1.Symbol, _ = util.GetSymbolWithAddress(tx1.Protocol)
	tx2.FromTransferEvent(evt, types.TX_TYPE_RECEIVE)
	tx2.Symbol = tx1.Symbol
	if err := processor.saveTransaction(&tx1); err != nil {
		return err
	}
	if err := processor.saveTransaction(&tx2); err != nil {
		return err
	}

	return nil
}

func (processor *AbiProcessor) handleApprovalEvent(input eventemitter.EventData) error {
	contractData := input.(EventData)
	if len(contractData.Topics) < 3 {
		log.Errorf("extractor,tx:%s approval event indexed fields number error", contractData.TxHash.Hex())
		return nil
	}

	contractEvent := contractData.Event.(*ethaccessor.ApprovalEvent)
	contractEvent.Owner = common.HexToAddress(contractData.Topics[1])
	contractEvent.Spender = common.HexToAddress(contractData.Topics[2])

	evt := contractEvent.ConvertDown()
	evt.TxInfo = contractData.TxInfo

	log.Debugf("extractor,tx:%s approval event owner:%s, spender:%s, value:%s", contractData.TxHash.Hex(), evt.Owner.Hex(), evt.Spender.Hex(), evt.Value.String())

	if processor.HasSpender(evt.Spender) {
		eventemitter.Emit(eventemitter.AccountApproval, evt)
	}

	return nil
}

func (processor *AbiProcessor) handleTokenRegisteredEvent(input eventemitter.EventData) error {
	contractData := input.(EventData)
	contractEvent := contractData.Event.(*ethaccessor.TokenRegisteredEvent)

	evt := contractEvent.ConvertDown()
	evt.TxInfo = contractData.TxInfo

	log.Debugf("extractor,tx:%s tokenRegistered event address:%s, symbol:%s", contractData.TxHash.Hex(), evt.Token.Hex(), evt.Symbol)

	eventemitter.Emit(eventemitter.TokenRegistered, evt)

	return nil
}

func (processor *AbiProcessor) handleTokenUnRegisteredEvent(input eventemitter.EventData) error {
	contractData := input.(EventData)
	contractEvent := contractData.Event.(*ethaccessor.TokenUnRegisteredEvent)

	evt := contractEvent.ConvertDown()
	evt.TxInfo = contractData.TxInfo

	log.Debugf("extractor,tx:%s tokenUnregistered event address:%s, symbol:%s", contractData.TxHash.Hex(), evt.Token.Hex(), evt.Symbol)

	eventemitter.Emit(eventemitter.TokenUnRegistered, evt)

	return nil
}

func (processor *AbiProcessor) handleAddressAuthorizedEvent(input eventemitter.EventData) error {
	contractData := input.(EventData)
	if len(contractData.Topics) < 2 {
		log.Errorf("extractor,tx:%s addressAuthorized event indexed fields number error", contractData.TxHash.Hex())
		return nil
	}

	contractEvent := contractData.Event.(*ethaccessor.AddressAuthorizedEvent)
	contractEvent.ContractAddress = common.HexToAddress(contractData.Topics[1])

	evt := contractEvent.ConvertDown()
	evt.TxInfo = contractData.TxInfo

	log.Debugf("extractor,tx:%s addressAuthorized event address:%s, number:%d", contractData.TxHash.Hex(), evt.Protocol.Hex(), evt.Number)

	eventemitter.Emit(eventemitter.AddressAuthorized, evt)

	return nil
}

func (processor *AbiProcessor) handleAddressDeAuthorizedEvent(input eventemitter.EventData) error {
	contractData := input.(EventData)
	if len(contractData.Topics) < 2 {
		log.Errorf("extractor,tx:%s addressDeAuthorized event indexed fields number error", contractData.TxHash.Hex())
		return nil
	}

	contractEvent := contractData.Event.(*ethaccessor.AddressDeAuthorizedEvent)
	contractEvent.ContractAddress = common.HexToAddress(contractData.Topics[1])

	evt := contractEvent.ConvertDown()
	evt.TxInfo = contractData.TxInfo

	log.Debugf("extractor,tx:%s addressDeAuthorized event address:%s, number:%d", contractData.TxHash.Hex(), evt.Protocol.Hex(), evt.Number)

	eventemitter.Emit(eventemitter.AddressAuthorized, evt)

	return nil
}

func (processor *AbiProcessor) handleEthTransfer(tx *ethaccessor.Transaction, receipt *ethaccessor.TransactionReceipt, time *big.Int, status uint8) error {
	var (
		dst      types.TransferEvent
		tx1, tx2 types.Transaction
	)

	dst.From = common.HexToAddress(tx.From)
	dst.To = common.HexToAddress(tx.To)
	dst.TxHash = common.HexToHash(tx.Hash)
	dst.Value = tx.Value.BigInt()
	dst.LogIndex = 0
	dst.Protocol = types.NilAddress
	dst.Symbol = "ETH"
	dst.BlockNumber = tx.BlockNumber.BigInt()
	dst.BlockTime = time.Int64()
	dst.Status = status

	dst.GasLimit = tx.Gas.BigInt()
	dst.GasPrice = tx.GasPrice.BigInt()
	dst.Nonce = tx.Nonce.BigInt()

	if receipt == nil {
		dst.GasUsed = big.NewInt(0)
	} else {
		dst.GasUsed = receipt.GasUsed.BigInt()
	}

	dst.Sender = common.HexToAddress(tx.From)
	dst.Receiver = common.HexToAddress(tx.To)
	tx1.FromTransferEvent(&dst, types.TX_TYPE_SEND)
	if err := processor.saveTransaction(&tx1); err != nil {
		return err
	}

	dst.Sender = common.HexToAddress(tx.To)
	dst.Receiver = common.HexToAddress(tx.From)
	tx2.FromTransferEvent(&dst, types.TX_TYPE_RECEIVE)
	if err := processor.saveTransaction(&tx2); err != nil {
		return err
	}

	// todo(fuk): emit to account manager
	return nil
}

func (processor *AbiProcessor) saveTransaction(tx *types.Transaction) error {
	var model dao.Transaction

	tx.CreateTime = tx.BlockTime
	tx.UpdateTime = tx.UpdateTime

	model.ConvertDown(tx)
	if unlocked, _ := processor.accountmanager.HasUnlocked(tx.Owner.Hex()); unlocked == true {
		return processor.db.SaveTransaction(&model)
	}

	return nil
}
