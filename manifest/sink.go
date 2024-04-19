package manifest

import (
	"archive/zip"
	"bytes"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	protov1 "github.com/golang/protobuf/proto"
	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/dynamic"
	pbss "github.com/streamingfast/substreams/pb/sf/substreams"
	pbssv1 "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/known/anypb"
)

func (r *manifestConverter) loadSinkConfig(pkg *pbssv1.Package, m *Manifest) error {
	if m.Sink == nil {
		return nil
	}
	if m.Sink.Module == "" {
		return fmt.Errorf(`sink: "module" unspecified`)
	}
	if m.Sink.Type == "" {
		return fmt.Errorf(`sink: "type" unspecified`)
	}
	pkg.SinkModule = m.Sink.Module

	msgDesc, err := getMsgDesc(m.Sink.Type, pkg.ProtoFiles)
	if err != nil {
		return err
	}

	jsonConfig, err := convertYAMLtoJSONCompat(m.Sink.Config, m.resolvePath, "", fieldResolver(msgDesc))
	if err != nil {
		return fmt.Errorf("converting YAML to JSON: %w", err)
	}
	jsonConfigBytes, err := json.Marshal(jsonConfig)
	if err != nil {
		return fmt.Errorf("marshalling config to JSON: %w", err)
	}

	dynConf := dynamic.NewMessageFactoryWithDefaults().NewDynamicMessage(msgDesc)
	if err := dynConf.UnmarshalJSON(jsonConfigBytes); err != nil {
		return fmt.Errorf("cannot unmarshal the SinkConfig into type %s. Is your YAML file valid ? %w", m.Sink.Type, err)
	}
	r.sinkConfigDynamicMessage = dynConf
	pbBytes, err := dynConf.Marshal()
	if err != nil {
		return fmt.Errorf("encoding protobuf from dynamic message: %w", err)
	}
	pkg.SinkConfig = &anypb.Any{
		TypeUrl: m.Sink.Type,
		Value:   pbBytes,
	}

	return nil
}

func getFieldsAndValues(dynMsg *dynamic.Message) (out []*fieldAndValue, err error) {
	for _, fd := range dynMsg.GetMessageDescriptor().GetFields() {
		field := &fieldAndValue{
			key: fd.GetName(),
		}
		if opts := fd.GetFieldOptions(); opts != nil {
			if val := opts.ProtoReflect().Get(pbss.E_Options.TypeDescriptor()); val.IsValid() {
				field.opts = val.Message().Interface().(*pbss.FieldOptions)
			}
		}

		val, err := dynMsg.TryGetField(fd)
		if err != nil {
			return nil, err
		}
		if mt := fd.GetMessageType(); mt != nil {

			switch val := val.(type) {
			case proto.Message:
				msgV1 := protov1.MessageV1(val)

				subDynMsg, err := dynamic.AsDynamicMessage(msgV1)
				if err != nil {
					return nil, err
				}
				v, err := getFieldsAndValues(subDynMsg)
				if err != nil {
					return nil, err
				}
				field.value = v
			case *dynamic.Message:
				field.value = val
			}
		} else {
			field.value = val
		}
		out = append(out, field)
	}
	return
}

// DescribeSinkConfigs returns a human-readable description of the sinkconfigs.
// Fields that were imported from files are returned as bytes in a map
func DescribeSinkConfigs(pkg *pbssv1.Package) (desc string, files map[string][]byte, err error) {
	if pkg.SinkConfig == nil {
		return "", nil, nil
	}

	msgDesc, err := getMsgDesc(pkg.SinkConfig.TypeUrl, pkg.ProtoFiles)
	if err != nil {
		return "", nil, err
	}

	dynMsg := dynamic.NewMessageFactoryWithDefaults().NewDynamicMessage(msgDesc)
	val := pkg.SinkConfig.Value
	if err := dynMsg.Unmarshal(val); err != nil {
		return "", nil, err
	}

	fields, err := getFieldsAndValues(dynMsg)
	if err != nil {
		return "", nil, err
	}

	desc, files = fieldDescriptions(fields, 0)
	return desc, files, nil
}

type fieldAndValue struct {
	key   string
	value interface{}
	opts  *pbss.FieldOptions
}

func fieldDescriptions(fields []*fieldAndValue, offset int) (string, map[string][]byte) {

	var out string
	outfiles := make(map[string][]byte)

	var prefix string
	for i := 0; i < offset; i++ {
		prefix += " "
	}

	for _, fv := range fields {
		switch val := fv.value.(type) {
		case []*fieldAndValue:
			textBlock, extraFiles := fieldDescriptions(val, offset+2)
			out += fmt.Sprintf("%s- %s:\n", prefix, fv.key) + textBlock
			for filename, content := range extraFiles {
				outfiles[fv.key+"_"+filename] = content
			}

		default:
			text, fullContent := fv.Describe(prefix)
			if fullContent != nil {
				outfiles[fv.key] = fullContent
			}
			out += text + "\n"
		}
	}

	return out, outfiles
}

// Describe returns the field values as a string, except for fields that were extracted from a file. (with options 'read_from_file or zip_from_folder')
// The latter will show a short description and return the full content as bytes.
func (f *fieldAndValue) Describe(prefix string) (string, []byte) {

	if f.opts != nil && (f.opts.LoadFromFile || f.opts.ZipFromFolder) { // special treatment for fields coming from files: show md5sum, return rawdata as bytes
		var rawdata []byte
		switch val := f.value.(type) {
		case string:
			rawdata = []byte(val)
		case []byte:
			rawdata = val
		}
		if len(rawdata) == 0 {
			return fmt.Sprintf(prefix+"- %v: (empty) %v", f.key, optsToString(f.opts)), nil
		}

		hasher := md5.New()
		hasher.Write(rawdata)
		sum := hex.EncodeToString(hasher.Sum(nil))

		return fmt.Sprintf(prefix+"- %v: (%d bytes) MD5SUM: %v %v", f.key, len(rawdata), sum, optsToString(f.opts)), rawdata
	}

	switch val := f.value.(type) {
	case []byte:
		if len(val) == 0 {
			return fmt.Sprintf(prefix+"- %v: (empty) %v", f.key, optsToString(f.opts)), nil
		}
		return fmt.Sprintf(prefix+"- %v: %v (hex-encoded) %v", f.key, hex.EncodeToString(val), optsToString(f.opts)), nil
	}

	return fmt.Sprintf(prefix+"- %v: %v %v", f.key, f.value, optsToString(f.opts)), nil
}

func optsToString(opts *pbss.FieldOptions) string {
	if opts == nil {
		return ""
	}
	if opts.LoadFromFile {
		return "[LOADED_FILE]"
	}
	if opts.ZipFromFolder {
		return "[ZIPPED_FOLDER]"
	}
	return ""
}

func fieldResolver(msgDesc *desc.MessageDescriptor) func(string) (opts *pbss.FieldOptions, isBytes bool) {
	return func(name string) (opts *pbss.FieldOptions, isBytes bool) {
		return resolve(name, msgDesc)
	}
}

func resolve(name string, msgDesc *desc.MessageDescriptor) (opts *pbss.FieldOptions, isBytes bool) {
	target := msgDesc.GetFullyQualifiedName() + "." + name
	for _, fd := range msgDesc.GetFields() {
		fqdn := fd.GetFullyQualifiedName()
		if fqdn == target {
			isBytes := fd.GetType() == descriptorpb.FieldDescriptorProto_TYPE_BYTES
			if opts := fd.GetFieldOptions(); opts != nil {
				if val := opts.ProtoReflect().Get(pbss.E_Options.TypeDescriptor()); val.IsValid() {
					options := val.Message().Interface().(*pbss.FieldOptions)
					return options, isBytes
				}
			}
			return &pbss.FieldOptions{}, false
		}
		if strings.HasPrefix(target, fqdn) {
			msgDesc = fd.GetMessageType()
			name = strings.TrimPrefix(target, fqdn+".")
			return resolve(name, msgDesc)
		}
	}
	return &pbss.FieldOptions{}, false
}

func getMsgDesc(anyType string, protoFiles []*descriptorpb.FileDescriptorProto) (*desc.MessageDescriptor, error) {
	files, err := desc.CreateFileDescriptors(protoFiles)
	if err != nil {
		return nil, fmt.Errorf("failed to create file descriptor: %w", err)
	}

	for _, file := range files {
		for _, msgDesc := range file.GetMessageTypes() {
			if msgDesc.GetFullyQualifiedName() == anyType {
				return msgDesc, nil
			}
		}
	}
	return nil, fmt.Errorf("sink: type: could not find protobuf message type %q in bundled protobuf descriptors", anyType)
}

func appendScope(prev, in string) string {
	if prev == "" {
		return in
	}
	return prev + "." + in
}

func convertYAMLtoJSONCompat(i any, resolvePath func(in string) string, scope string, resolveField func(string) (opts *pbss.FieldOptions, isBytes bool)) (out any, err error) {
	switch x := i.(type) {
	case map[interface{}]interface{}:
		m2 := map[string]interface{}{}
		for k, v := range x {
			kk := k.(string)
			m2[kk], err = convertYAMLtoJSONCompat(v, resolvePath, appendScope(scope, kk), resolveField)
			if err != nil {
				return nil, err
			}
		}
		return m2, nil
	case map[string]interface{}:
		m2 := map[string]interface{}{}
		for k, v := range x {
			m2[k], err = convertYAMLtoJSONCompat(v, resolvePath, appendScope(scope, k), resolveField)
			if err != nil {
				return nil, err
			}
		}
		return m2, nil
	case []interface{}:
		for i, v := range x {
			x[i], err = convertYAMLtoJSONCompat(v, resolvePath, scope, resolveField)
			if err != nil {
				return nil, err
			}
		}
	case string:
		opts, isBytes := resolveField(scope)

		if opts.LoadFromFile {

			if strings.HasPrefix(x, "@@") { // support previous behavior
				x = x[1:]
			}

			if strings.HasPrefix(x, "@") { // support previous behavior
				x = x[1:]
			}

			cnt, err := os.ReadFile(resolvePath(x))
			if err != nil {
				return nil, fmt.Errorf("%s (field loaded from file): could not read file %q: %w", scope, x, err)
			}
			if isBytes {
				return base64.StdEncoding.EncodeToString(cnt), nil
			}
			return string(cnt), nil
		}

		if opts.ZipFromFolder {
			if !isBytes {
				return "", fmt.Errorf("invalid field %q: option zip_from_folder is set on a field that is not of type Bytes", scope)
			}

			var buf bytes.Buffer
			w := zip.NewWriter(&buf)
			if err := addFiles(w, resolvePath(x)); err != nil {
				w.Close()
				return "", err
			}
			if err := w.Close(); err != nil {
				return "", err
			}
			return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
		}

		switch {
		case strings.HasPrefix(x, "@@"):
			zlog.Warn("using deprecated prefix @@ to load binary file, use `(sf.substreams.v1.options).loadFromFile = true` in your protobuf definition", zap.String("scope", scope))
			cnt, err := os.ReadFile(resolvePath(x[2:]))
			if err != nil {
				return nil, fmt.Errorf("@@ notation: could not read %s: %w", x[2:], err)
			}
			return base64.StdEncoding.EncodeToString(cnt), nil
		case strings.HasPrefix(x, "@"):
			zlog.Warn("using deprecated prefix @ to load file, use `(sf.substreams.v1.options).loadFromFile = true` in your protobuf definition ", zap.String("scope", scope))
			cnt, err := os.ReadFile(resolvePath(x[1:]))
			if err != nil {
				return nil, fmt.Errorf("@ notation: could not read %s: %w", x[1:], err)
			}
			return string(cnt), nil
		}
	}
	return i, nil
}

func addFiles(w *zip.Writer, basePath string) error {
	return filepath.Walk(basePath, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		path = strings.TrimPrefix(path, basePath)                   // relative path
		path = strings.TrimPrefix(path, string(filepath.Separator)) // ensure we don't start with a slash
		path = strings.Replace(path, "\\", "/", -1)                 // w.Create does not support windows-style separators

		if info.IsDir() {
			path += fmt.Sprintf("%s%c", path, os.PathSeparator)
			_, err := w.Create(path)
			return err
		}

		f, err := w.Create(path)
		if err != nil {
			return err
		}

		in, err := os.Open(filepath.Join(basePath, path))
		if err != nil {
			return err
		}
		_, err = io.Copy(f, in)
		if err != nil {
			return err
		}
		return nil
	})
}
