package output

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/alecthomas/chroma/quick"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/dynamic"
	"google.golang.org/protobuf/types/known/anypb"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func (o *Output) renderPayload(in *pbsubstreams.ModuleOutput) string {
	out := &strings.Builder{}

	for _, log := range in.DebugLogs {
		out.WriteString(Styles.LogLabel.Render("log: "))
		out.WriteString(Styles.LogLine.Render(log))
		out.WriteString("\n")
	}

	if len(in.DebugLogs) != 0 {
		out.WriteString("\n")
	}

	switch data := in.Data.(type) {
	case *pbsubstreams.ModuleOutput_MapOutput:
		if len(data.MapOutput.Value) != 0 {
			msgDesc := o.msgDescs[in.Name]
			out.WriteString(o.decodeDynamicMessage(msgDesc, data.MapOutput))
		}
	case *pbsubstreams.ModuleOutput_DebugStoreDeltas:
		if len(data.DebugStoreDeltas.Deltas) != 0 {
			//out.WriteString(o.decodeDynamicStoreDeltas())
			//s = append(s, ui.renderDecoratedDeltas(in.Name, data.DebugStoreDeltas.Deltas, false)...)
		} else {
			out.WriteString("No deltas")
		}
	}
	return out.String()
}

func (o *Output) decodeDynamicMessage(msgDesc *desc.MessageDescriptor, anyin *anypb.Any) string {
	if msgDesc == nil {
		return Styles.ErrorLine.Render(fmt.Sprintf("Unknown type: %s\n", anyin.MessageName()))
	}
	msgType := msgDesc.GetFullyQualifiedName()
	in := anyin.GetValue()
	dynMsg := dynamic.NewMessageFactoryWithDefaults().NewDynamicMessage(msgDesc)
	if err := dynMsg.Unmarshal(in); err != nil {
		return Styles.ErrorLine.Render(
			fmt.Sprintf("Failed unmarshalling message into %s: %s\n%s",
				msgType,
				err.Error(),
				decodeAsString(in),
			),
		)
	}

	cnt, err := dynMsg.MarshalJSONIndent()
	if err != nil {
		return Styles.ErrorLine.Render(
			fmt.Sprintf("Failed marshalling into JSON: %s\nString representation: %s",
				err.Error(),
				decodeAsString(in),
			),
		)
	}

	out := &strings.Builder{}
	if err := quick.Highlight(out, string(cnt), "json", "terminal256", "dracula"); err != nil {
		return string(cnt)
	}
	return out.String()
}

//
//func (o *Output) decodeDynamicStoreDeltas(msgDesc *desc.MessageDescriptor, in []byte) string {
//	if msgDesc == nil {
//		return Styles.ErrorLine.Render(fmt.Sprintf("Unknown type: %s\n", anyin.MessageName()))
//	}
//	msgType := msgDesc.GetFullyQualifiedName()
//	if msgType == "bytes" {
//		return []byte(decodeAsHex(in))
//	}
//
//	if msgDesc != nil {
//		dynMsg := dynamic.NewMessageFactoryWithDefaults().NewDynamicMessage(msgDesc)
//		if err := dynMsg.Unmarshal(in); err != nil {
//			cnt, _ := json.Marshal(&ErrorWrap{
//				Error:  fmt.Sprintf("error unmarshalling message into %s: %s\n", msgDesc.GetFullyQualifiedName(), err.Error()),
//				String: string(decodeAsString(in)),
//				Bytes:  in,
//			})
//			return cnt
//		}
//		cnt, err := msgDescToJSON(msgType, blockNum, modName, dynMsg, false)
//		if err != nil {
//			cnt, _ := json.Marshal(&ErrorWrap{
//				Error:  fmt.Sprintf("error encoding protobuf %s into json: %s\n", msgDesc.GetFullyQualifiedName(), err),
//				String: string(decodeAsString(in)),
//				Bytes:  in,
//			})
//			return decodeAsString(cnt)
//		}
//		return cnt
//	}
//
//	// default, other msgType: "bigint", "bigfloat", "int64", "float64", "string":
//	return decodeAsString(in)
//}

func decodeAsString(in []byte) []byte { return []byte(fmt.Sprintf("%q", string(in))) }
func decodeAsHex(in []byte) string    { return "(hex) " + hex.EncodeToString(in) }
