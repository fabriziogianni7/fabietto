package abi

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestEncodeERC20Approve(t *testing.T) {
	spender := common.HexToAddress("0x2222222222222222222222222222222222222222")
	amount := big.NewInt(1_000_000)
	data, err := EncodeERC20Approve(spender, amount)
	if err != nil {
		t.Fatal(err)
	}
	wantSel := common.FromHex("0x095ea7b3")
	if len(data) < 4 || data[0] != wantSel[0] || data[1] != wantSel[1] || data[2] != wantSel[2] || data[3] != wantSel[3] {
		t.Fatalf("selector got %x want %x", data[:4], wantSel)
	}
	zero, err := EncodeERC20Approve(spender, big.NewInt(0))
	if err != nil || len(zero) < 4 {
		t.Fatalf("approve zero: %v %x", err, zero)
	}
}

func TestEncodeERC20Transfer(t *testing.T) {
	recipient := common.HexToAddress("0x34eBF0f9104a21ac09Bf02542930F37Fe4cfB490")
	amount := big.NewInt(100000) // 0.1 USDC with 6 decimals
	data, err := EncodeERC20Transfer(recipient, amount)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 4 {
		t.Fatalf("calldata too short")
	}
	wantSel := common.FromHex("0xa9059cbb")
	if data[0] != wantSel[0] || data[1] != wantSel[1] || data[2] != wantSel[2] || data[3] != wantSel[3] {
		t.Fatalf("selector got %x want %x", data[:4], wantSel)
	}
}

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
