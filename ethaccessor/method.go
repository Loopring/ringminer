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

package ethaccessor

import (
	"errors"
	"fmt"
	"github.com/Loopring/relay/log"
	"github.com/Loopring/relay/types"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	ethTypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/rpc"
	"math/big"
	"time"
)

func (accessor *EthNodeAccessor) Erc20Balance(tokenAddress, ownerAddress common.Address, blockParameter string) (*big.Int, error) {
	var balance types.Big
	callMethod := accessor.ContractCallMethod(accessor.Erc20Abi, tokenAddress)
	if err := callMethod(&balance, "balanceOf", blockParameter, ownerAddress); nil != err {
		return nil, err
	} else {
		return balance.BigInt(), err
	}
}

func (accessor *EthNodeAccessor) Erc20Allowance(tokenAddress, ownerAddress, spenderAddress common.Address, blockParameter string) (*big.Int, error) {
	var allowance types.Big
	callMethod := accessor.ContractCallMethod(accessor.Erc20Abi, tokenAddress)
	if err := callMethod(&allowance, "allowance", blockParameter, ownerAddress, spenderAddress); nil != err {
		return nil, err
	} else {
		return allowance.BigInt(), err
	}
}

func (accessor *EthNodeAccessor) GetCancelledOrFilled(contractAddress common.Address, orderhash common.Hash, blockNumStr string) (*big.Int, error) {
	var amount types.Big
	contractAbi, ok := accessor.ProtocolImpls[contractAddress]
	if !ok {
		return nil, errors.New("accessor: contract address invalid -> " + contractAddress.Hex())
	}
	callMethod := accessor.ContractCallMethod(contractAbi.ProtocolImplAbi, contractAddress)
	if err := callMethod(&amount, "cancelledOrFilled", blockNumStr, orderhash); err != nil {
		return nil, err
	}

	return amount.BigInt(), nil
}

func (accessor *EthNodeAccessor) GetCutoff(contractAddress common.Address, owner common.Address, blockNumStr string) (int, error) {
	var cutoff int
	contractAbi, ok := accessor.ProtocolImpls[contractAddress]
	if !ok {
		return cutoff, errors.New("accessor: contract address invalid -> " + contractAddress.Hex())
	}
	callMethod := accessor.ContractCallMethod(contractAbi.ProtocolImplAbi, contractAddress)
	if err := callMethod(&cutoff, "cutoffs", blockNumStr, owner); err != nil {
		return cutoff, err
	}

	return cutoff, nil
}

func (accessor *EthNodeAccessor) BatchErc20BalanceAndAllowance(reqs []*BatchErc20Req) error {
	reqElems := make([]rpc.BatchElem, 2*len(reqs))
	erc20Abi := accessor.Erc20Abi

	for idx, req := range reqs {
		balanceOfData, _ := erc20Abi.Pack("balanceOf", req.Owner)
		balanceOfArg := &CallArg{}
		balanceOfArg.To = req.Token
		balanceOfArg.Data = common.ToHex(balanceOfData)

		allowanceData, _ := erc20Abi.Pack("allowance", req.Owner, req.Spender)
		allowanceArg := &CallArg{}
		allowanceArg.To = req.Token
		allowanceArg.Data = common.ToHex(allowanceData)
		reqElems[2*idx] = rpc.BatchElem{
			Method: "eth_call",
			Args:   []interface{}{balanceOfArg, req.BlockParameter},
			Result: &req.Balance,
		}
		reqElems[2*idx+1] = rpc.BatchElem{
			Method: "eth_call",
			Args:   []interface{}{allowanceArg, req.BlockParameter},
			Result: &req.Allowance,
		}
	}

	if err := accessor.Client.BatchCall(reqElems); err != nil {
		return err
	}

	for idx, req := range reqs {
		req.BalanceErr = reqElems[2*idx].Error
		req.AllowanceErr = reqElems[2*idx+1].Error
	}
	return nil
}

func (accessor *EthNodeAccessor) EstimateGas(callData []byte, to common.Address) (gas, gasPrice *big.Int, err error) {
	var gasBig, gasPriceBig types.Big
	if err = accessor.Call(&gasPriceBig, "eth_gasPrice"); nil != err {
		return
	}
	callArg := &CallArg{}
	callArg.To = to
	callArg.Data = common.ToHex(callData)
	callArg.GasPrice = gasPriceBig
	if err = accessor.Call(&gasBig, "eth_estimateGas", callArg); nil != err {
		return
	}
	gasPrice = gasPriceBig.BigInt()
	gas = gasBig.BigInt()
	return
}

func (accessor *EthNodeAccessor) ContractCallMethod(a *abi.ABI, contractAddress common.Address) func(result interface{}, methodName, blockParameter string, args ...interface{}) error {
	return func(result interface{}, methodName string, blockParameter string, args ...interface{}) error {
		if callData, err := a.Pack(methodName, args...); nil != err {
			return err
		} else {
			arg := &CallArg{}
			arg.From = contractAddress
			arg.To = contractAddress
			arg.Data = common.ToHex(callData)
			return accessor.Call(result, "eth_call", arg, blockParameter)
		}
	}
}

func (ethAccessor *EthNodeAccessor) SignAndSendTransaction(result interface{}, sender accounts.Account, tx *ethTypes.Transaction) error {
	var err error
	if tx, err = ethAccessor.ks.SignTx(sender, tx, nil); nil != err {
		return err
	}
	if txData, err := rlp.EncodeToBytes(tx); nil != err {
		return err
	} else {
		log.Debugf("txhash:%s, value:%s, gas:%s, gasPrice:%s", tx.Hash().Hex(), tx.Value().String(), tx.Gas().String(), tx.GasPrice().String())
		err = ethAccessor.Call(result, "eth_sendRawTransaction", common.ToHex(txData))
		return err
	}
}

func (accessor *EthNodeAccessor) ContractSendTransactionByData(sender accounts.Account, to common.Address, gas, gasPrice *big.Int, callData []byte) (string, error) {
	if nil == gasPrice || gasPrice.Cmp(big.NewInt(0)) <= 0 {
		return "", errors.New("gasPrice must be setted.")
	}

	if nil == gas || gas.Cmp(big.NewInt(0)) <= 0 {
		return "", errors.New("gas must be setted.")
	}
	var txHash string
	var nonce types.Big
	if err := accessor.Call(&nonce, "eth_getTransactionCount", sender.Address.Hex(), "pending"); nil != err {
		return "", err
	}
	transaction := ethTypes.NewTransaction(nonce.Uint64(),
		common.HexToAddress(to.Hex()),
		big.NewInt(0),
		gas,
		gasPrice,
		callData)
	if err := accessor.SignAndSendTransaction(&txHash, sender, transaction); nil != err {
		return "", err
	} else {
		return txHash, err
	}
}

func (accessor *EthNodeAccessor) ContractSendTransactionMethod(a abi.ABI, contractAddress common.Address) func(sender accounts.Account, methodName string, gas, gasPrice *big.Int, args ...interface{}) (string, error) {
	return func(sender accounts.Account, methodName string, gas, gasPrice *big.Int, args ...interface{}) (string, error) {
		if callData, err := a.Pack(methodName, args...); nil != err {
			return "", err
		} else {
			return accessor.ContractSendTransactionByData(sender, contractAddress, gas, gasPrice, callData)
		}
	}
}

func (iterator *BlockIterator) Next() (interface{}, error) {
	var block interface{}
	if iterator.withTxData {
		block = &BlockWithTxObject{}
	} else {
		block = &BlockWithTxHash{}
	}
	if nil != iterator.endNumber && iterator.endNumber.Cmp(big.NewInt(0)) > 0 && iterator.endNumber.Cmp(iterator.currentNumber) < 0 {
		return nil, errors.New("finished")
	}

	var blockNumber types.Big
	if err := iterator.ethClient.Call(&blockNumber, "eth_blockNumber"); nil != err {
		return nil, err
	} else {
		confirmNumber := iterator.currentNumber.Uint64() + iterator.confirms
		if blockNumber.Uint64() < confirmNumber {
		hasNext:
			for {
				select {
				// todo(fk):modify this duration
				case <-time.After(time.Duration(5 * time.Second)):
					if err1 := iterator.ethClient.Call(&blockNumber, "eth_blockNumber"); nil == err1 && blockNumber.Uint64() >= confirmNumber {
						break hasNext
					}
				}
			}
		}
	}

	if err := iterator.ethClient.Call(&block, "eth_getBlockByNumber", fmt.Sprintf("%#x", iterator.currentNumber), iterator.withTxData); nil != err {
		return nil, err
	} else {
		iterator.currentNumber.Add(iterator.currentNumber, big.NewInt(1))
		return block, nil
	}
}

func (iterator *BlockIterator) Prev() (interface{}, error) {
	var block interface{}
	if iterator.withTxData {
		block = &BlockWithTxObject{}
	} else {
		block = &BlockWithTxHash{}
	}
	if nil != iterator.startNumber && iterator.startNumber.Cmp(big.NewInt(0)) > 0 && iterator.startNumber.Cmp(iterator.currentNumber) > 0 {
		return nil, errors.New("finished")
	}
	prevNumber := new(big.Int).Sub(iterator.currentNumber, big.NewInt(1))
	if err := iterator.ethClient.Call(&block, "eth_getBlockByNumber", fmt.Sprintf("%#x", prevNumber), iterator.withTxData); nil != err {
		return nil, err
	} else {
		if nil == block {
			return nil, errors.New("there isn't a block with number:" + prevNumber.String())
		}
		iterator.currentNumber.Sub(iterator.currentNumber, big.NewInt(1))
		return block, nil
	}
}

func (ethAccessor *EthNodeAccessor) BlockIterator(startNumber, endNumber *big.Int, withTxData bool, confirms uint64) *BlockIterator {
	iterator := &BlockIterator{
		startNumber:   new(big.Int).Set(startNumber),
		endNumber:     endNumber,
		currentNumber: new(big.Int).Set(startNumber),
		ethClient:     ethAccessor,
		withTxData:    withTxData,
		confirms:      confirms,
	}
	return iterator
}

func (ethAccessor *EthNodeAccessor) GetSenderAddress(protocol common.Address) (common.Address, error) {
	impl, ok := ethAccessor.ProtocolImpls[protocol]
	if !ok {
		return common.Address{}, errors.New("accessor method:invalid protocol address")
	}

	return impl.DelegateAddress, nil
}
