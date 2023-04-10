package output

import (
	"encoding/hex"
	"fmt"
	"log"
	"strings"

	"github.com/alecthomas/chroma/quick"
	"github.com/jhump/protoreflect/dynamic"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

func (o *Output) renderPayload(in *pbsubstreams.ModuleOutput) string {
	if in == nil {
		return ""
	}
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
		if len(data.DebugStoreDeltas.Deltas) == 0 {
			out.WriteString("No deltas")
		} else {
			msgDesc := o.msgDescs[in.Name]
			out.WriteString(o.decodeDynamicStoreDeltas(data.DebugStoreDeltas.Deltas, msgDesc))
			//s = append(s, ui.renderDecoratedDeltas(in.Name, data.DebugStoreDeltas.Deltas, false)...)
		}
	}
	return out.String()
}

func (o *Output) decodeDynamicMessage(msgDesc *manifest.ModuleDescriptor, anyin *anypb.Any) string {
	if msgDesc.MessageDescriptor == nil {
		// TODO: also add the bytes message, and rotate the format with `f`
		return Styles.ErrorLine.Render(fmt.Sprintf("Unknown type: %s\n", anyin.MessageName()))
	}
	in := anyin.GetValue()
	dynMsg := dynamic.NewMessageFactoryWithDefaults().NewDynamicMessage(msgDesc.MessageDescriptor)
	if err := dynMsg.Unmarshal(in); err != nil {
		return Styles.ErrorLine.Render(
			fmt.Sprintf("Failed unmarshalling message into %s: %s\n%s",
				msgDesc.ProtoMessageType,
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

	return highlightJSON(string(cnt))
}

func highlightJSON(in string) string {
	out := &strings.Builder{}
	if err := quick.Highlight(out, in, "json", "terminal256", "dracula"); err != nil {
		return in
	}
	return out.String()
}

func (o *Output) decodeDynamicStoreDeltas(deltas []*pbsubstreams.StoreDelta, msgDesc *manifest.ModuleDescriptor) string {
	out := &strings.Builder{}
	for _, delta := range deltas {
		out.WriteString(fmt.Sprintf("%s (%d) KEY: %q\n", delta.Operation, delta.Ordinal, delta.Key))
		out.WriteString(o.decodeDelta(delta.OldValue, msgDesc, "OLD"))
		out.WriteString(o.decodeDelta(delta.NewValue, msgDesc, "NEW"))

		out.WriteString("\n")
	}
	return out.String()
}

func decodeAsString(in []byte) []byte { return []byte(fmt.Sprintf("%q", string(in))) }
func decodeAsHex(in []byte) string    { return "(hex) " + hex.EncodeToString(in) }

func decodeAsType(in []byte, typ string) string {
	switch typ {
	case "bytes":
		return decodeAsHex(in)
	default:
		return string(in)
	}
}

func (o *Output) decodeDelta(in []byte, msgDesc *manifest.ModuleDescriptor, oldNew string) string {
	out := &strings.Builder{}
	out.WriteString(fmt.Sprintf("  %s: ", oldNew))

	if len(in) == 0 {
		out.WriteString("(none)\n")
	} else if msgDesc.MessageDescriptor == nil {
		out.WriteString(fmt.Sprintf("%q\n", decodeAsType(in, msgDesc.StoreValueType)))
	} else {

		msg := o.messageFactory.NewDynamicMessage(msgDesc.MessageDescriptor)
		if err := msg.Unmarshal(in); err != nil {
			log.Println("error unmarshalling message:", err)
		} else {
			jsonBytes, err := msg.MarshalJSONIndent()
			if err != nil {
				out.WriteString("failed to marshal json: " + err.Error() + ", hex:")
				out.WriteString(decodeAsHex(in))
			} else {
				jsonStr := strings.Replace(string(jsonBytes), "\n", "\n  ", -1)
				jsonStr = highlightJSON(jsonStr)
				out.WriteString(jsonStr)
			}
			out.WriteString("\n")
		}
	}
	return out.String()
}

func applySearchColoring(content, highlight string) (string, int, []int) {
	highlight = strings.TrimSpace(highlight)
	if highlight == "" {
		return content, 0, nil
	}

	var positions []int
	lines := strings.Split(content, "\n")
	newLines := make([]string, len(lines))
	var totalCount int
	for lineNo, line := range lines {
		count := strings.Count(line, highlight)
		totalCount += count
		if count != 0 {
			newLines = append(newLines, strings.ReplaceAll(line, highlight, "\033[48;5;11m"+highlight+"\033[0m"))
			positions = append(positions, lineNo)
		} else {
			newLines = append(newLines, line)
		}
	}
	return strings.Join(newLines, "\n"), totalCount, positions
}
