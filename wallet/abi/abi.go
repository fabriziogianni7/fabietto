package abi

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

// EncodeCall encodes a contract call from ABI JSON and method + args.
func EncodeCall(abiJSON, method string, args ...interface{}) ([]byte, error) {
	parsed, err := abi.JSON(strings.NewReader(abiJSON))
	if err != nil {
		return nil, fmt.Errorf("parse abi: %w", err)
	}
	data, err := parsed.Pack(method, args...)
	if err != nil {
		return nil, fmt.Errorf("pack %s: %w", method, err)
	}
	return data, nil
}

// DecodeCall decodes calldata using the given ABI and method.
func DecodeCall(abiJSON, method string, data []byte) (map[string]interface{}, error) {
	parsed, err := abi.JSON(strings.NewReader(abiJSON))
	if err != nil {
		return nil, fmt.Errorf("parse abi: %w", err)
	}
	m, ok := parsed.Methods[method]
	if !ok {
		return nil, fmt.Errorf("method %s not found", method)
	}
	vals, err := m.Inputs.Unpack(data)
	if err != nil {
		return nil, fmt.Errorf("unpack: %w", err)
	}
	out := make(map[string]interface{})
	for i, arg := range m.Inputs {
		if i < len(vals) {
			out[arg.Name] = vals[i]
		}
	}
	return out, nil
}

// DecodeCallHex is like DecodeCall but accepts hex-encoded calldata.
func DecodeCallHex(abiJSON, method, hexData string) (map[string]interface{}, error) {
	data, err := hex.DecodeString(strings.TrimPrefix(hexData, "0x"))
	if err != nil {
		return nil, err
	}
	return DecodeCall(abiJSON, method, data)
}

// ERC20BalanceOfSelector is the 4-byte selector for balanceOf(address).
var ERC20BalanceOfSelector = common.FromHex("0x70a08231")

// EncodeERC20BalanceOf encodes balanceOf(account).
func EncodeERC20BalanceOf(account common.Address) []byte {
	a, _ := abi.JSON(strings.NewReader(erc20BalanceOfABI))
	data, _ := a.Pack("balanceOf", account)
	return data
}

const erc20BalanceOfABI = `[{"constant":true,"inputs":[{"name":"_owner","type":"address"}],"name":"balanceOf","outputs":[{"name":"balance","type":"uint256"}],"type":"function"}]`

// erc20TransferABI is minimal ERC-20 transfer(address,uint256) for encoding only.
const erc20TransferABI = `[{"constant":false,"inputs":[{"name":"to","type":"address"},{"name":"amount","type":"uint256"}],"name":"transfer","outputs":[{"name":"","type":"bool"}],"type":"function"}]`

const erc20DecimalsABI = `[{"constant":true,"inputs":[],"name":"decimals","outputs":[{"name":"","type":"uint8"}],"type":"function"}]`

// EncodeERC20Transfer encodes transfer(recipient, amount) for standard ERC-20 tokens.
func EncodeERC20Transfer(recipient common.Address, amount *big.Int) ([]byte, error) {
	if amount == nil {
		return nil, fmt.Errorf("amount is nil")
	}
	if amount.Sign() <= 0 {
		return nil, fmt.Errorf("amount must be positive")
	}
	parsed, err := abi.JSON(strings.NewReader(erc20TransferABI))
	if err != nil {
		return nil, fmt.Errorf("parse erc20 transfer abi: %w", err)
	}
	data, err := parsed.Pack("transfer", recipient, amount)
	if err != nil {
		return nil, fmt.Errorf("pack transfer: %w", err)
	}
	return data, nil
}

// erc20ApproveABI is minimal ERC-20 approve(address,uint256) for encoding only.
const erc20ApproveABI = `[{"constant":false,"inputs":[{"name":"spender","type":"address"},{"name":"amount","type":"uint256"}],"name":"approve","outputs":[{"name":"","type":"bool"}],"type":"function"}]`

// EncodeERC20Approve encodes approve(spender, amount) for standard ERC-20 tokens.
// amount may be zero to revoke allowance.
func EncodeERC20Approve(spender common.Address, amount *big.Int) ([]byte, error) {
	if amount == nil {
		return nil, fmt.Errorf("amount is nil")
	}
	if amount.Sign() < 0 {
		return nil, fmt.Errorf("amount must be non-negative")
	}
	parsed, err := abi.JSON(strings.NewReader(erc20ApproveABI))
	if err != nil {
		return nil, fmt.Errorf("parse erc20 approve abi: %w", err)
	}
	data, err := parsed.Pack("approve", spender, amount)
	if err != nil {
		return nil, fmt.Errorf("pack approve: %w", err)
	}
	return data, nil
}

// EncodeERC20Decimals encodes decimals() calldata.
func EncodeERC20Decimals() ([]byte, error) {
	parsed, err := abi.JSON(strings.NewReader(erc20DecimalsABI))
	if err != nil {
		return nil, err
	}
	return parsed.Pack("decimals")
}

// DecodeERC20BalanceReturn decodes the return data of balanceOf from eth_call.
func DecodeERC20BalanceReturn(data []byte) (*big.Int, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty return data")
	}
	parsed, err := abi.JSON(strings.NewReader(erc20BalanceOfABI))
	if err != nil {
		return nil, err
	}
	vals, err := parsed.Unpack("balanceOf", data)
	if err != nil {
		return nil, err
	}
	if len(vals) != 1 {
		return nil, fmt.Errorf("balanceOf: expected 1 value, got %d", len(vals))
	}
	bi, ok := vals[0].(*big.Int)
	if !ok {
		return nil, fmt.Errorf("balanceOf: unexpected type %T", vals[0])
	}
	return bi, nil
}

// DecodeERC20DecimalsReturn decodes the return data of decimals() from eth_call.
func DecodeERC20DecimalsReturn(data []byte) (uint8, error) {
	if len(data) == 0 {
		return 0, fmt.Errorf("empty return data")
	}
	parsed, err := abi.JSON(strings.NewReader(erc20DecimalsABI))
	if err != nil {
		return 0, err
	}
	vals, err := parsed.Unpack("decimals", data)
	if err != nil {
		return 0, err
	}
	if len(vals) != 1 {
		return 0, fmt.Errorf("decimals: expected 1 value, got %d", len(vals))
	}
	switch v := vals[0].(type) {
	case uint8:
		return v, nil
	case *big.Int:
		return uint8(v.Uint64()), nil
	default:
		return 0, fmt.Errorf("decimals: unexpected type %T", vals[0])
	}
}
