package bytes

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/btcsuite/btcutil/base58"
)

// Encoding represents the different encoding types for protobuf bytes fields
type Encoding int

const (
	// EncodingRaw keeps bytes as raw binary data (default)
	EncodingRaw Encoding = iota
	// EncodingHex encodes bytes as hexadecimal string
	EncodingHex
	// EncodingHexWith0x encodes bytes as hexadecimal string with 0x prefix
	EncodingHexWith0x
	// EncodingBase64 encodes bytes as base64 string
	EncodingBase64
	// EncodingBase58 encodes bytes as base58 string
	EncodingBase58
)

// String returns the string representation of the encoding
func (e Encoding) String() string {
	switch e {
	case EncodingRaw:
		return "raw"
	case EncodingHex:
		return "hex"
	case EncodingHexWith0x:
		return "0xhex"
	case EncodingBase64:
		return "base64"
	case EncodingBase58:
		return "base58"
	default:
		return "unknown"
	}
}

// ParseEncoding parses a string into an Encoding type
func ParseEncoding(s string) (Encoding, error) {
	switch strings.ToLower(s) {
	case "raw":
		return EncodingRaw, nil
	case "hex":
		return EncodingHex, nil
	case "0xhex":
		return EncodingHexWith0x, nil
	case "base64":
		return EncodingBase64, nil
	case "base58":
		return EncodingBase58, nil
	default:
		return EncodingRaw, fmt.Errorf("invalid encoding: %s", s)
	}
}

// IsStringType returns true if the encoding converts bytes to string database type
func (e Encoding) IsStringType() bool {
	return e != EncodingRaw
}

// EncodeBytes encodes the given bytes using the specified encoding
func (e Encoding) EncodeBytes(data []byte) (interface{}, error) {
	switch e {
	case EncodingRaw:
		return data, nil
	case EncodingHex:
		return hex.EncodeToString(data), nil
	case EncodingHexWith0x:
		return "0x" + hex.EncodeToString(data), nil
	case EncodingBase64:
		return base64.StdEncoding.EncodeToString(data), nil
	case EncodingBase58:
		return base58.Encode(data), nil
	default:
		return nil, fmt.Errorf("unsupported encoding: %s", e)
	}
}

// DecodeBytes decodes the given string back to bytes using the specified encoding
func (e Encoding) DecodeBytes(encoded interface{}) ([]byte, error) {
	switch e {
	case EncodingRaw:
		if data, ok := encoded.([]byte); ok {
			return data, nil
		}
		return nil, fmt.Errorf("expected []byte for raw encoding, got %T", encoded)
	case EncodingHex:
		if str, ok := encoded.(string); ok {
			return hex.DecodeString(str)
		}
		return nil, fmt.Errorf("expected string for hex encoding, got %T", encoded)
	case EncodingHexWith0x:
		if str, ok := encoded.(string); ok {
			if strings.HasPrefix(str, "0x") || strings.HasPrefix(str, "0X") {
				return hex.DecodeString(str[2:])
			}
			return hex.DecodeString(str)
		}
		return nil, fmt.Errorf("expected string for 0xhex encoding, got %T", encoded)
	case EncodingBase64:
		if str, ok := encoded.(string); ok {
			return base64.StdEncoding.DecodeString(str)
		}
		return nil, fmt.Errorf("expected string for base64 encoding, got %T", encoded)
	case EncodingBase58:
		if str, ok := encoded.(string); ok {
			return base58.Decode(str), nil
		}
		return nil, fmt.Errorf("expected string for base58 encoding, got %T", encoded)
	default:
		return nil, fmt.Errorf("unsupported encoding: %s", e)
	}
}
