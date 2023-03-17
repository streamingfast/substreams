package manifest

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/dynamic"
	"google.golang.org/protobuf/types/known/anypb"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func (r *Reader) loadSinkConfig(pkg *pbsubstreams.Package, m *Manifest) error {
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
	jsonConfig, err := convertYAMLtoJSONCompat(m.Sink.Config, m.resolvePath)
	if err != nil {
		return fmt.Errorf("sink: config: converting to json: %w", err)
	}
	jsonConfigBytes, err := json.Marshal(jsonConfig)
	if err != nil {
		return fmt.Errorf("sink: config: error marshalling to json: %w", err)
	}

	r.sinkConfigJSON = string(jsonConfigBytes)

	files, err := desc.CreateFileDescriptors(pkg.ProtoFiles)
	if err != nil {
		return fmt.Errorf("failed to create file descriptor: %w", err)
	}

	var found bool
files:
	for _, file := range files {
		for _, msgDesc := range file.GetMessageTypes() {
			if msgDesc.GetFullyQualifiedName() == m.Sink.Type {
				dynConf := dynamic.NewMessageFactoryWithDefaults().NewDynamicMessage(msgDesc)
				if err := dynConf.UnmarshalJSON(jsonConfigBytes); err != nil {
					return fmt.Errorf("sink: config: encoding json into protobuf message: %w", err)
				}
				r.sinkConfigDynamicMessage = dynConf
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

func convertYAMLtoJSONCompat(i any, resolvePath func(in string) string) (out any, err error) {
	switch x := i.(type) {
	case map[interface{}]interface{}:
		m2 := map[string]interface{}{}
		for k, v := range x {
			m2[k.(string)], err = convertYAMLtoJSONCompat(v, resolvePath)
			if err != nil {
				return nil, err
			}
		}
		return m2, nil
	case map[string]interface{}:
		m2 := map[string]interface{}{}
		for k, v := range x {
			m2[k], err = convertYAMLtoJSONCompat(v, resolvePath)
			if err != nil {
				return nil, err
			}
		}
		return m2, nil
	case []interface{}:
		for i, v := range x {
			x[i], err = convertYAMLtoJSONCompat(v, resolvePath)
			if err != nil {
				return nil, err
			}
		}
	case string:
		switch {
		case strings.HasPrefix(x, "@@"):
			cnt, err := os.ReadFile(resolvePath(x[2:]))
			if err != nil {
				return nil, fmt.Errorf("@@ notation: could not read %s: %w", x[2:], err)
			}
			return base64.StdEncoding.EncodeToString(cnt), nil
		case strings.HasPrefix(x, "@"):
			cnt, err := os.ReadFile(resolvePath(x[1:]))
			if err != nil {
				return nil, fmt.Errorf("@ notation: could not read %s: %w", x[1:], err)
			}
			return string(cnt), nil
		}
	}
	return i, nil
}
