package protodecode

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/golang/protobuf/jsonpb"
	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/dynamic"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"google.golang.org/protobuf/types/known/anypb"
)

type OutputStreamPattern struct {
	pattern string
	regex   *regexp.Regexp
}

func NewOutputStreamPattern(pattern string) OutputStreamPattern {
	regex, err := regexp.Compile(pattern)
	if err != nil {
		return OutputStreamPattern{pattern: pattern, regex: nil}
	}

	return OutputStreamPattern{pattern: pattern, regex: regex}
}

func (o *OutputStreamPattern) Matches(input string) bool {
	if o.regex == nil {
		return o.pattern == input
	}

	return o.regex.MatchString(input)
}

type Decoder struct {
	msgDescs    map[string]*desc.MessageDescriptor
	msgTypes    map[string]string
	anyResolver *pbsubstreams.PackageAnyResolver
}

func NewDecoder(pkg *pbsubstreams.Package, outputStreamNames []string) (*Decoder, error) {
	anyResolver, err := pkg.NewAnyResolver()
	if err != nil {
		return nil, fmt.Errorf("new any resolver: %w", err)
	}

	decoder := &Decoder{
		msgDescs:    map[string]*desc.MessageDescriptor{},
		msgTypes:    map[string]string{},
		anyResolver: anyResolver,
	}

	fileDescs, err := desc.CreateFileDescriptors(pkg.ProtoFiles)
	if err != nil {
		return nil, fmt.Errorf("couldn't convert file descriptors: %w", err)
	}

	patterns := make([]OutputStreamPattern, len(outputStreamNames))
	for i, name := range outputStreamNames {
		patterns[i] = NewOutputStreamPattern(name)
	}

	for _, mod := range pkg.Modules.Modules {
		for _, outputStreamPattern := range patterns {
			if outputStreamPattern.Matches(mod.Name) {
				var msgType string
				switch modKind := mod.Kind.(type) {
				case *pbsubstreams.Module_KindStore_:
					msgType = modKind.KindStore.ValueType
				case *pbsubstreams.Module_KindMap_:
					msgType = modKind.KindMap.OutputType
				case *pbsubstreams.Module_KindBlockIndex_:
					msgType = modKind.KindBlockIndex.OutputType
				}

				msgType = strings.TrimPrefix(msgType, "proto:")
				decoder.msgTypes[mod.Name] = msgType

				var msgDesc *desc.MessageDescriptor
				for _, file := range fileDescs {
					msgDesc = file.FindMessage(msgType)
					if msgDesc != nil {
						break
					}
				}
				decoder.msgDescs[mod.Name] = msgDesc
			}
		}
	}

	return decoder, nil
}

func (d *Decoder) DecodeDynamicMessage(msgType string, msgDesc *desc.MessageDescriptor, blockNum uint64, modName string, anyin *anypb.Any) []byte {
	in := anyin.GetValue()
	if msgDesc == nil {
		cnt, _ := json.Marshal(&UnknownWrap{
			Module:      modName,
			UnknownType: string(anyin.MessageName()),
			String:      string(decodeAsString(in)),
			Bytes:       in,
		})
		return cnt
	}
	dynMsg := dynamic.NewMessageFactoryWithDefaults().NewDynamicMessage(msgDesc)
	if err := dynMsg.Unmarshal(in); err != nil {
		cnt, _ := json.Marshal(&ErrorWrap{
			Module: modName,
			Error:  fmt.Sprintf("error unmarshalling message into %s: %s\n", msgType, err.Error()),
			String: string(decodeAsString(in)),
			Bytes:  in,
		})
		return cnt
	}

	cnt, err := d.msgDescToJSON(msgType, blockNum, modName, dynMsg, true)
	if err != nil {
		cnt, _ := json.Marshal(&ErrorWrap{
			Module: modName,
			Error:  fmt.Sprintf("error encoding protobuf %s into json: %s\n", msgType, err),
			String: string(decodeAsString(in)),
			Bytes:  in,
		})
		return decodeAsString(cnt)
	}

	return cnt
}

func (d *Decoder) DecodeDynamicStoreDeltas(msgType string, msgDesc *desc.MessageDescriptor, blockNum uint64, modName string, in []byte) []byte {
	if msgType == "bytes" {
		return []byte(decodeAsHex(in))
	}

	if msgDesc != nil {
		dynMsg := dynamic.NewMessageFactoryWithDefaults().NewDynamicMessage(msgDesc)
		if err := dynMsg.Unmarshal(in); err != nil {
			cnt, _ := json.Marshal(&ErrorWrap{
				Error:  fmt.Sprintf("error unmarshalling message into %s: %s\n", msgDesc.GetFullyQualifiedName(), err.Error()),
				String: string(decodeAsString(in)),
				Bytes:  in,
			})
			return cnt
		}
		cnt, err := d.msgDescToJSON(msgType, blockNum, modName, dynMsg, false)
		if err != nil {
			cnt, _ := json.Marshal(&ErrorWrap{
				Error:  fmt.Sprintf("error encoding protobuf %s into json: %s\n", msgDesc.GetFullyQualifiedName(), err),
				String: string(decodeAsString(in)),
				Bytes:  in,
			})
			return decodeAsString(cnt)
		}
		return cnt
	}

	// default, other msgType: "bigint", "bigfloat", "int64", "float64", "string":
	return decodeAsString(in)
}

func (d *Decoder) GetMessageDescriptor(modName string) *desc.MessageDescriptor {
	return d.msgDescs[modName]
}

func (d *Decoder) GetMessageType(modName string) string {
	return d.msgTypes[modName]
}

func (d *Decoder) HasMessageType(modName string) bool {
	_, ok := d.msgTypes[modName]
	return ok
}

func (d *Decoder) msgDescToJSON(
	msgType string,
	blockNum uint64,
	mod string,
	dynMsg *dynamic.Message,
	wrap bool,
) ([]byte, error) {
	cnt, err := dynMsg.MarshalJSONPB(&jsonpb.Marshaler{
		AnyResolver: d.anyResolver,
	})
	if err != nil {
		return nil, fmt.Errorf("protojson marshal: %w", err)
	}

	if wrap {
		wrappedCnt, err := json.Marshal(ModuleWrap{
			Module:   mod,
			BlockNum: blockNum,
			Type:     msgType,
			Data:     cnt,
		})
		if err != nil {
			return nil, err
		}

		return wrappedCnt, nil
	}

	return cnt, nil
}

// Helper types
type UnknownWrap struct {
	Module      string `json:"@module"`
	UnknownType string `json:"@unknown"`
	String      string `json:"@str"`
	Bytes       []byte `json:"@bytes"`
}

type ErrorWrap struct {
	Module string `json:"@module,omitempty"`
	Error  string `json:"@error"`
	String string `json:"@str"`
	Bytes  []byte `json:"@bytes"`
}

type ModuleWrap struct {
	Module   string          `json:"@module"`
	BlockNum uint64          `json:"@block"`
	Type     string          `json:"@type"`
	Data     json.RawMessage `json:"@data"`
}

// Helper functions
func decodeAsString(in []byte) []byte { return []byte(fmt.Sprintf("%q", string(in))) }
func decodeAsHex(in []byte) string    { return "(hex) " + hex.EncodeToString(in) }
