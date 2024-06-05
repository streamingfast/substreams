package wasm

import (
	"errors"
	"fmt"
	"strings"
)

var ErrUnknownRuntimeExtension = errors.New("unknown wasm runtime extension")

type RuntimeExtensionID string

const (
	RuntimeExtensionIDWASMBindgenShims RuntimeExtensionID = "wasm-bindgen-shims"
)

var runtimeExtensionIDs = map[RuntimeExtensionID]struct{}{
	RuntimeExtensionIDWASMBindgenShims: {},
}

type RuntimeExtensions []RuntimeExtension

func (extensions RuntimeExtensions) Has(id RuntimeExtensionID) bool {
	for _, ext := range extensions {
		if ext.ID == id {
			return true
		}
	}
	return false
}

type RuntimeExtension struct {
	ID    RuntimeExtensionID
	Value *string
}

func ParseRuntimeExtensions(expressions string) (extensions RuntimeExtensions, err error) {
	for _, expr := range strings.Split(expressions, ",") {
		id, value, hasValue := strings.Cut(strings.TrimSpace(expr), "=")
		if _, found := runtimeExtensionIDs[RuntimeExtensionID(id)]; !found {
			return nil, fmt.Errorf("%w: %s", ErrUnknownRuntimeExtension, id)
		}

		var valuePtr *string
		if hasValue {
			valuePtr = &value
		}

		extensions = append(extensions, RuntimeExtension{
			ID:    RuntimeExtensionID(id),
			Value: valuePtr,
		})
	}

	return
}
