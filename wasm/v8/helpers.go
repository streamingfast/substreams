package v8

import (
	"fmt"

	"rogchap.com/v8go"
)

// We can check if theres a better way to do this but I couldn't find another way
func ByteArrayToJSArray(b []byte) string {
	if len(b) == 0 {
		return "[]"
	}

	// 3 is the max length of decimal value so 255 (allocate 3 chars max)
	js := make([]byte, 0, len(b)*3)
	js = append(js, '[')
	for index, val := range b {
		if index > 0 {
			js = append(js, ',')
		}
		js = append(js, fmt.Sprint(val)...)
	}
	js = append(js, ']')
	return string(js)
}

// InjectUint8Array returns JavaScript code that assigns a Uint8Array to globalThis
func InjectUint8Array(varName string, data []byte) string {
	return fmt.Sprintf("globalThis.%s = new Uint8Array(%s);", varName, ByteArrayToJSArray(data))
}

// reads a Uint8Array from a v8go Object and returns its bytes
func ExtractUint8Array(obj *v8go.Object) ([]byte, error) {
	lengthValue, _ := obj.Get("length")
	length := int(lengthValue.Integer())
	if length == 0 {
		return nil, nil
	}

	bytes := make([]byte, length)

	for index := range length {
		elem, _ := obj.GetIdx(uint32(index))
		if !elem.IsNumber() {
			return nil, fmt.Errorf("Uint8Array[%d] is not a number", index)
		}
		bytes[index] = byte(elem.Integer())
	}
	return bytes, nil
}
