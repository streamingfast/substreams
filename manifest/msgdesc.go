package manifest

import (
	"fmt"
	"strings"

	"github.com/jhump/protoreflect/desc"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type ModuleDescriptor struct {
	// Either or
	StoreValueType string
	MapOutputType  string

	ProtoMessageType  string
	MessageDescriptor *desc.MessageDescriptor
}

func BuildMessageDescriptors(pkg *pbsubstreams.Package) (out map[string]*ModuleDescriptor, err error) {
	fileDescs, err := desc.CreateFileDescriptors(pkg.ProtoFiles)
	if err != nil {
		return nil, fmt.Errorf("couldn't convert, should do this check much earlier: %w", err)
	}

	out = map[string]*ModuleDescriptor{}
	for _, mod := range pkg.Modules.Modules {
		desc := &ModuleDescriptor{}
		var msgType string
		switch modKind := mod.Kind.(type) {
		case *pbsubstreams.Module_KindStore_:
			msgType = modKind.KindStore.ValueType
			desc.StoreValueType = msgType
		case *pbsubstreams.Module_KindMap_:
			msgType = modKind.KindMap.OutputType
			desc.MapOutputType = msgType
		}
		if strings.HasPrefix(msgType, "proto:") {
			msgType = strings.TrimPrefix(msgType, "proto:")
			desc.ProtoMessageType = msgType
			for _, file := range fileDescs {
				desc.MessageDescriptor = file.FindMessage(msgType)
				if desc.MessageDescriptor != nil {
					break
				}
			}
		}

		//log.Printf("Module %s store %s map %s protomsg: %s ptr: %T\n", mod.Name, desc.StoreValueType, desc.MapOutputType, desc.ProtoMessageType, desc.MessageDescriptor)
		out[mod.Name] = desc
	}
	return
}
