package calldata

import (
	"encoding/hex"
	"fmt"

	"github.com/ethereum/go-ethereum/crypto"
)

var knownSelectors map[[4]byte]string

func init() {
	knownSelectors = make(map[[4]byte]string)
	for _, e := range []struct {
		sig  string
		name string
	}{
		{"approve(address,uint256)", "approve"},
		{"transfer(address,uint256)", "transfer"},
		{"transferFrom(address,address,uint256)", "transferFrom"},
		{"increaseAllowance(address,uint256)", "increaseAllowance"},
		{"permit(address,address,uint256,uint256,uint8,uint8,uint256)", "permit"},
	} {
		var k [4]byte
		h := crypto.Keccak256([]byte(e.sig))
		copy(k[:], h[:4])
		knownSelectors[k] = e.name
	}
}

// MethodNameFromSelector returns a canonical method name for known 4-byte selectors, else "".
func MethodNameFromSelector(selector []byte) string {
	if len(selector) < 4 {
		return ""
	}
	var k [4]byte
	copy(k[:], selector[:4])
	if m, ok := knownSelectors[k]; ok {
		return m
	}
	return ""
}

// MethodNameFromData reads the first 4 bytes of calldata as the selector.
func MethodNameFromData(data []byte) string {
	return MethodNameFromSelector(data)
}

// SelectorHex returns 0x-prefixed 8-char hex for the first 4 bytes of data.
func SelectorHex(data []byte) string {
	if len(data) < 4 {
		return ""
	}
	return "0x" + hex.EncodeToString(data[:4])
}

// AnnotateAction sets action.Method from calldata when empty (contract_call only).
func AnnotateAction(method *string, data []byte) {
	if method == nil {
		return
	}
	if *method != "" {
		return
	}
	if m := MethodNameFromData(data); m != "" {
		*method = m
		return
	}
	if len(data) >= 4 {
		*method = fmt.Sprintf("unknown_%s", SelectorHex(data))
	}
}
