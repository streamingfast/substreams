// Code generated stub for compilation purposes only.
package pbsql

import "google.golang.org/protobuf/reflect/protoreflect"

type Service struct {
	Schema string `protobuf:"bytes,1,opt,name=schema,proto3" json:"schema,omitempty"`
}

func (x *Service) Reset()                        {}
func (x *Service) String() string                { return x.Schema }
func (x *Service) ProtoMessage()                 {}
func (x *Service) ProtoReflect() protoreflect.Message { return nil }
