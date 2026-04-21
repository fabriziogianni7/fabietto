package abi

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestEncodeDecodeERC20BalanceOf(t *testing.T) {
	owner := common.HexToAddress("0x1111111111111111111111111111111111111111")
	data := EncodeERC20BalanceOf(owner)
	if len(data) < 4 {
		t.Fatalf("calldata too short")
	}
	// eth_call return: uint256 ABI word
	ret := common.LeftPadBytes(big.NewInt(1500000).Bytes(), 32)
	got, err := DecodeERC20BalanceReturn(ret)
	if err != nil {
		t.Fatal(err)
	}
	if got.Cmp(big.NewInt(1500000)) != 0 {
		t.Fatalf("got %s", got)
	}
	_ = data
}
