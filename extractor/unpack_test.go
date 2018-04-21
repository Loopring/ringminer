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

package extractor_test

import (
	"github.com/Loopring/relay/ethaccessor"
	"github.com/Loopring/relay/test"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"math/big"
	"testing"
)

func init() {
	c := test.Cfg()
	println(c.Owner.Name)
}

func TestExtractorServiceImpl_UnpackSubmitRingMethod(t *testing.T) {
	input := "0x0fd2f4910000000000000000000000000000000000000000000000000000000000000120000000000000000000000000000000000000000000000000000000000000020000000000000000000000000000000000000000000000000000000000000003e0000000000000000000000000000000000000000000000000000000000000044000000000000000000000000000000000000000000000000000000000000004a000000000000000000000000000000000000000000000000000000000000005600000000000000000000000000000000000000000000000000000000000000620000000000000000000000000000000000000000000000000000000000000000100000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000002000000000000000000000000b94065482ad64d4c2b9252358d746b39e820a582000000000000000000000000b5f64747127be058ee7239b363269fc8cf3f4a8700000000000000000000000066d965aa92b77a99e30e5c69b531a5ef3009bcb000000000000000000000000023bd9cafe75610c3185b85bc59f760f400bd89b5000000000000000000000000f5b3b365fa319342e89a3da71ba393e12d9f63c3000000000000000000000000f93a6d1c19874ae8fe88db0cbfee3d65eb70fe86000000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000003635c9adc5dea00000000000000000000000000000000000000000000000000000000000746a528800000000000000000000000000000000000000000000000000000000005ad5ca5e000000000000000000000000000000000000000000000000000000005ad71bde00000000000000000000000000000000000000000000000005b09cd3e5e9000000000000000000000000000000000000000000000000000f0aee097a09a57e4900000000000000000000000000000000000000000000000000000000000000010000000000000000000000000000000000000000000000000000090548bf8f00000000000000000000000000000000000000000000000052cd2f884aa2900000000000000000000000000000000000000000000000000000000000005ad5ab10000000000000000000000000000000000000000000000000000000005ad6fc90000000000000000000000000000000000000000000000000117c6b5300fe000000000000000000000000000000000000000000000000000000000280cce3de9e00000000000000000000000000000000000000000000000000000000000000010000000000000000000000000000000000000000000000000000000000000002000000000000000000000000000000000000000000000000000000000000006400000000000000000000000000000000000000000000000000000000000000320000000000000000000000000000000000000000000000000000000000000002000000000000000000000000000000000000000000000000000000000000000100000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000005000000000000000000000000000000000000000000000000000000000000001b000000000000000000000000000000000000000000000000000000000000001b000000000000000000000000000000000000000000000000000000000000001c000000000000000000000000000000000000000000000000000000000000001c000000000000000000000000000000000000000000000000000000000000001b00000000000000000000000000000000000000000000000000000000000000051c8183706a4ddb6a9e277a5ba8c8d1ddc3802c7d164025f2832073182de111ab86d64c5992138f472db2ffe106761a06a695b288ffd80e4be0c608b39defa6fce06da1a12fe0562c2a84f7a273421f7c13ea8c7535a7d630b0169c1014897cfa8bd6cc23903b3df26d733c2f8027a03f2df057fb24a12faebe62c78e87f2e81ebab519af63933e55b6beb4b878f27138d0b30db399257afc874ab9a4ebcc249800000000000000000000000000000000000000000000000000000000000000057377648c7b18f3cf2ec2f2ec08fda57abb4659ea6e890f809150730a0d4624777e5795e8c4d1514868d771129f924e17236ef7ffd20a465820cde465b810f5fd31bb82755ac4f0aa547d799f8bf82eca74356f1315d395a82873759bd90f72753f2e391c062ddcf8581fd10b05c1eb02d06c1f77af91dc77aab9f52971b73b9e6269d44098cc78e7612e1eb86d56142dc91c2c317cb1e0dc00ce5423da0574b7"

	var ring ethaccessor.SubmitRingMethod

	data := hexutil.MustDecode("0x" + input[10:])

	if err := ethaccessor.ProtocolImplAbi().UnpackMethodInput(&ring, "submitRing", data); err != nil {
		t.Fatalf(err.Error())
	}

	orders, err := ring.ConvertDown()
	if err != nil {
		t.Fatalf(err.Error())
	}

	for k, v := range orders {
		t.Log(k, "tokenS", v.TokenS.Hex())
		t.Log(k, "tokenB", v.TokenB.Hex())

		t.Log(k, "amountS", v.AmountS.String())
		t.Log(k, "amountB", v.AmountB.String())
		t.Log(k, "validSince", v.ValidSince.String())
		t.Log(k, "validUntil", v.ValidUntil.String())
		t.Log(k, "lrcFee", v.LrcFee.String())
		t.Log(k, "rateAmountS", ring.UintArgsList[k][5].String())

		t.Log(k, "marginSplitpercentage", v.MarginSplitPercentage)
		t.Log(k, "feeSelectionList", ring.Uint8ArgsList[k][0])

		t.Log(k, "buyNoMoreThanAmountB", v.BuyNoMoreThanAmountB)

		t.Log(k, "v", v.V)
		t.Log(k, "s", v.S.Hex())
		t.Log(k, "r", v.R.Hex())
	}

	t.Log("feeSelection", ring.FeeSelections)
}

func TestExtractorServiceImpl_UnpackWethWithdrawalMethod(t *testing.T) {
	input := "0x2e1a7d4d0000000000000000000000000000000000000000000000000000000000000064"

	var withdrawal ethaccessor.WethWithdrawalMethod

	data := hexutil.MustDecode("0x" + input[10:])

	if err := ethaccessor.WethAbi().UnpackMethodInput(&withdrawal.Value, "withdraw", data); err != nil {
		t.Fatalf(err.Error())
	}

	evt := withdrawal.ConvertDown()
	t.Logf("withdrawal event value:%s", evt.Value)
}

func TestExtractorServiceImpl_UnpackCancelOrderMethod(t *testing.T) {
	input := "0x8c59f7ca000000000000000000000000b1018949b241d76a1ab2094f473e9befeabb5ead000000000000000000000000480037780d0b0e766941b8c5e99e685bf8812c39000000000000000000000000f079e0612e869197c5f4c7d0a95df570b163232b000000000000000000000000b1018949b241d76a1ab2094f473e9befeabb5ead00000000000000000000000047fe1648b80fa04584241781488ce4c0aaca23e400000000000000000000000000000000000000000000003635c9adc5dea00000000000000000000000000000000000000000000000000000016345785d8a0000000000000000000000000000000000000000000000000000000000005ad8a62f000000000000000000000000000000000000000000000000000000005b5c7c2f00000000000000000000000000000000000000000000000029a2241af62c00000000000000000000000000000000000000000000000000001bc16d674ec8000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000001b39026cca9b4e4e42ac957182e6bbeebd88d327c9368f905620b8edbf2be687af12e190eb0ec2fc5b337487834aeb9ce9df2f0275f281b3e7ca5bdec13246444f"

	var method ethaccessor.CancelOrderMethod

	data := hexutil.MustDecode("0x" + input[10:])

	//for i := 0; i < len(data)/32; i++ {
	//	t.Logf("index:%d -> %s", i, common.ToHex(data[i*32:(i+1)*32]))
	//}

	if err := ethaccessor.ProtocolImplAbi().UnpackMethodInput(&method, "cancelOrder", data); err != nil {
		t.Fatalf(err.Error())
	}

	order, cancelAmount, err := method.ConvertDown()
	if err != nil {
		t.Fatalf(err.Error())
	}

	order.DelegateAddress = common.HexToAddress("0xf49733091a3e1ddec740bca4c325f8aaee6ee307")
	order.Hash = order.GenerateHash()
	t.Log("de", order.DelegateAddress.Hex())
	t.Log("orderHash", order.Hash.Hex())
	t.Log("owner", order.Owner.Hex())
	t.Log("wallet", order.WalletAddress.Hex())
	t.Log("auth", order.AuthAddr.Hex())
	t.Log("tokenS", order.TokenS.Hex())
	t.Log("tokenB", order.TokenB.Hex())
	t.Log("amountS", order.AmountS.String())
	t.Log("amountB", order.AmountB.String())
	t.Log("validSince", order.ValidSince.String())
	t.Log("validUntil", order.ValidUntil.String())
	t.Log("lrcFee", order.LrcFee.String())
	t.Log("cancelAmount", method.OrderValues[5].String())
	t.Log("buyNoMoreThanAmountB", order.BuyNoMoreThanAmountB)
	t.Log("marginSplitpercentage", order.MarginSplitPercentage)
	t.Log("v", order.V)
	t.Log("s", order.S.Hex())
	t.Log("r", order.R.Hex())
	t.Log("cancelAmount", cancelAmount)
}

func TestExtractorServiceImpl_UnpackApproveMethod(t *testing.T) {
	input := "0x095ea7b300000000000000000000000045aa504eb94077eec4bf95a10095a8e3196fc5910000000000000000000000000000000000000000000000008ac7230489e80000"

	var method ethaccessor.ApproveMethod

	data := hexutil.MustDecode("0x" + input[10:])
	for i := 0; i < len(data)/32; i++ {
		t.Logf("index:%d -> %s", i, common.ToHex(data[i*32:(i+1)*32]))
	}

	if err := ethaccessor.Erc20Abi().UnpackMethodInput(&method, "approve", data); err != nil {
		t.Fatalf(err.Error())
	}

	approve := method.ConvertDown()
	t.Logf("approve spender:%s, value:%s", approve.Spender.Hex(), approve.Value.String())
}

func TestExtractorServiceImpl_UnpackTransferMethod(t *testing.T) {
	input := "0xa9059cbb0000000000000000000000008311804426a24495bd4306daf5f595a443a52e32000000000000000000000000000000000000000000000000000000174876e800"
	data := hexutil.MustDecode("0x" + input[10:])
	var method ethaccessor.TransferMethod
	if err := ethaccessor.Erc20Abi().UnpackMethodInput(&method, "transfer", data); err != nil {
		t.Fatalf(err.Error())
	}
	transfer := method.ConvertDown()
	t.Logf("transfer receiver:%s, value:%s", transfer.Receiver.Hex(), transfer.Value.String())
}

func TestExtractorServiceImpl_UnpackTransferEvent(t *testing.T) {
	inputs := []string{
		"0x00000000000000000000000000000000000000000000001d2666491321fc5651",
		"0x0000000000000000000000000000000000000000000000008ac7230489e80000",
		"0x0000000000000000000000000000000000000000000000004c0303a413a39039",
		"0x000000000000000000000000000000000000000000000000016345785d8a0000",
	}
	transfer := &ethaccessor.TransferEvent{}

	for _, input := range inputs {
		data := hexutil.MustDecode(input)

		if err := ethaccessor.Erc20Abi().Unpack(transfer, "Transfer", data, abi.SEL_UNPACK_EVENT); err != nil {
			t.Fatalf(err.Error())
		}

		t.Logf("transfer value:%s", transfer.Value.String())
	}
}

func TestExtractorServiceImpl_UnpackRingMined(t *testing.T) {
	input := "0x0000000000000000000000000000000000000000000000000000000000000003000000000000000000000000750ad4351bb728cec7d639a9511f9d6488f1e2590000000000000000000000003a49f1f84234615caa46e9c89ad3c53e8f142b6c00000000000000000000000000000000000000000000000000000000000000a000000000000000000000000000000000000000000000000000000000000001000000000000000000000000000000000000000000000000000000000000000002d5f3eb4410b337628be5372c9c3fc790bcef4113fcb229c296bd2519c41aaed2400e5da365eb5b208ccccc5ca8ed27da8286fbb76139cda77b907234ebd09a93000000000000000000000000000000000000000000000000000000000000000200000000000000000000000000000000000000000000000ae56f730e6d840000000000000000000000000000000000000000000000000000286c39e79fdbc4f8000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000001532b10660c30647ae00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000286c39e79fdbc4f800000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000001158b4151fad157c0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000ae56f730e6d840000"
	ringmined := &ethaccessor.RingMinedEvent{}

	data := hexutil.MustDecode(input)

	if err := ethaccessor.ProtocolImplAbi().Unpack(ringmined, "RingMined", data, abi.SEL_UNPACK_EVENT); err != nil {
		t.Fatalf(err.Error())
	}

	_, fills, err := ringmined.ConvertDown()
	if err != nil {
		t.Fatalf(err.Error())
	}
	for _, fill := range fills {
		t.Logf("amountS:%s, amountB:%s", fill.AmountS.String(), fill.AmountB.String())
	}
}

func TestExtractorServiceImpl_UnpackWethDeposit(t *testing.T) {
	input := "0x0000000000000000000000000000000000000000000000000de0b6b3a7640000"
	deposit := &ethaccessor.WethDepositEvent{}

	data := hexutil.MustDecode(input)

	if err := ethaccessor.WethAbi().Unpack(deposit, "Deposit", data, abi.SEL_UNPACK_EVENT); err != nil {
		t.Fatalf(err.Error())
	} else {
		t.Logf("deposit value:%s", deposit.Value.String())
	}
}

func TestExtractorServiceImpl_UnpackTokenRegistry(t *testing.T) {
	input := "0x000000000000000000000000f079e0612e869197c5f4c7d0a95df570b163232b0000000000000000000000000000000000000000000000000000000000000040000000000000000000000000000000000000000000000000000000000000000457455448"

	tokenRegistry := &ethaccessor.TokenRegisteredEvent{}

	data := hexutil.MustDecode(input)

	println("====token registry", len(data))

	if err := ethaccessor.WethAbi().Unpack(tokenRegistry, "TokenRegistered", data, abi.SEL_UNPACK_EVENT); err != nil {
		t.Fatalf(err.Error())
	} else {
		t.Logf("TokenRegistered symbol:%s, address:%s", tokenRegistry.Symbol, tokenRegistry.Token.Hex())
	}
}

func TestExtractorServiceImpl_UnpackTokenUnRegistry(t *testing.T) {
	input := "0x000000000000000000000000529540ee6862158f47d647ae023098f6705210a90000000000000000000000000000000000000000000000000000000000000040000000000000000000000000000000000000000000000000000000000000000457455448"

	tokenUnRegistry := &ethaccessor.TokenUnRegisteredEvent{}

	data := hexutil.MustDecode(input)

	println("====token unregistry", len(data))

	if err := ethaccessor.WethAbi().Unpack(tokenUnRegistry, "TokenUnregistered", data, abi.SEL_UNPACK_EVENT); err != nil {
		t.Fatalf(err.Error())
	} else {
		t.Logf("TokenUnregistered symbol:%s, address:%s", tokenUnRegistry.Symbol, tokenUnRegistry.Token.Hex())
	}
}

func TestExtractorServiceImpl_Compare(t *testing.T) {
	str1 := "547722557505166136913"
	str2 := "1000000000000000000000"
	num1, _ := big.NewInt(0).SetString(str1, 0)
	num2, _ := big.NewInt(0).SetString(str2, 0)
	if num1.Cmp(num2) > 0 {
		t.Logf("%s > %s", str1, str2)
	} else {
		t.Logf("%s <= %s", str1, str2)
	}
}
