package wasm

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/mr-tron/base58"
)

type PanicError struct {
	message      string
	filename     string
	lineNumber   int
	columnNumber int
}

func (e *PanicError) Error() string {
	return fmt.Sprintf("panic in the wasm: %q at %s:%d:%d", e.message, e.filename, e.lineNumber, e.columnNumber)
}

func NewPanicError(message, filename string, lineNumber, columnNumber int) *PanicError {
	return &PanicError{message, filename, lineNumber, columnNumber}
}

// Attempts to decode a hash string into bytes using multiple formats
func DecodeHashString(hashStr string) []byte {
	if hashStr == "" {
		return nil
	}

	// hex with 0x prefix
	if strings.HasPrefix(hashStr, "0x") && len(hashStr) == 66 {
		if decoded, err := hex.DecodeString(hashStr[2:]); err == nil {
			return decoded
		}
	}

	//hex without prefix
	if len(hashStr) == 64 {
		if decoded, err := hex.DecodeString(strings.ToLower(hashStr)); err == nil {
			return decoded
		}
	}

	// base58
	if decoded, err := base58.Decode(hashStr); err == nil && len(decoded) == 32 {
		return decoded
	}

	// Fallback: return nil if no format worked
	return nil
}
