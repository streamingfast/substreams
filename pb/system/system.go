package system

import (
	_ "embed"
)

//go:embed system.pb
var ProtobufDescriptors []byte
