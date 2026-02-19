package protodecode

import (
	"fmt"

	"google.golang.org/protobuf/proto"
)

type PreferredVTCodec struct{}

type vtprotoMessage interface {
	MarshalVT() ([]byte, error)
	UnmarshalVT([]byte) error
}

func (PreferredVTCodec) Marshal(v any) ([]byte, error) {
	vt, ok := v.(vtprotoMessage)
	if !ok {
		nonvt, ok := v.(proto.Message)
		if ok {
			return proto.Marshal(nonvt)
		}
		return nil, fmt.Errorf("failed to marshal, message is %T (missing proto or vtprotobuf helpers)", v)
	}
	return vt.MarshalVT()
}

func (PreferredVTCodec) Unmarshal(data []byte, v any) error {
	vt, ok := v.(vtprotoMessage)
	if !ok {
		nonvt, ok := v.(proto.Message)
		if ok {
			return proto.Unmarshal(data, nonvt)
		}

		return fmt.Errorf("failed to unmarshal, message is %T (missing proto or vtprotobuf helpers)", v)
	}
	return vt.UnmarshalVT(data)
}

func (PreferredVTCodec) Name() string {
	return "proto"
}
