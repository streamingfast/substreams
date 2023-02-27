package manifest

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/dynamic"
	"google.golang.org/protobuf/types/known/anypb"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func loadSinkConfig(pkg *pbsubstreams.Package, m *Manifest, protoDescs []*desc.FileDescriptor) error {
	if m.Sink == nil {
		return nil
	}
	if m.Sink.Module == "" {
		return errors.New(`sink: "module" unspecified`)
	}
	if m.Sink.Type == "" {
		return errors.New(`sink: "type" unspecified`)
	}
	pkg.SinkModule = m.Sink.Module
	jsonConfig := convertYAMLtoJSONCompat(m.Sink.Config)
	jsonConfigBytes, err := json.Marshal(jsonConfig)
	if err != nil {
		return fmt.Errorf("sink: config: error marshalling to json: %w", err)
	}

	var found bool
files:
	for _, file := range protoDescs {
		for _, msgDesc := range file.GetMessageTypes() {
			fmt.Println("Type found:", file.GetName(), msgDesc.GetFullyQualifiedName())
			if msgDesc.GetFullyQualifiedName() == m.Sink.Type {
				// TODO: create a dynamic message of that type
				// unpack the JSON into it
				// serialize and create the `anypb.Any`
				// assign it to pkg.SinkConfig
				dynConf := dynamic.NewMessageFactoryWithDefaults().NewDynamicMessage(msgDesc)
				if err := dynConf.UnmarshalJSON(jsonConfigBytes); err != nil {
					return fmt.Errorf("sink: config: encoding json into protobuf message: %w", err)
				}
				pbBytes, err := dynConf.Marshal()
				if err != nil {
					return fmt.Errorf("sink: config: encoding protobuf from dynamic message: %w", err)
				}
				pkg.SinkConfig = &anypb.Any{
					TypeUrl: m.Sink.Type,
					Value:   pbBytes,
				}
				found = true
				break files
			}
		}
	}
	if !found {
		return fmt.Errorf("sink: type: could not find protobuf message type %q in bundled protobuf descriptors", m.Sink.Type)
	}

	return nil
}

func convertYAMLtoJSONCompat(i interface{}) interface{} {
	switch x := i.(type) {
	case map[interface{}]interface{}:
		m2 := map[string]interface{}{}
		for k, v := range x {
			m2[k.(string)] = convertYAMLtoJSONCompat(v)
		}
		return m2
	case map[string]interface{}:
		m2 := map[string]interface{}{}
		for k, v := range x {
			m2[k] = convertYAMLtoJSONCompat(v)
		}
		return m2
	case []interface{}:
		for i, v := range x {
			x[i] = convertYAMLtoJSONCompat(v)
		}
	}
	return i
}
