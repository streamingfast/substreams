package output

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/alecthomas/chroma/quick"
	"github.com/charmbracelet/lipgloss"
	"github.com/itchyny/gojq"
	"github.com/jhump/protoreflect/dynamic"

	"github.com/golang/protobuf/jsonpb"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"

	"github.com/muesli/termenv"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/streamingfast/substreams/manifest"
)

func (o *Output) wrapLogs(log string) string {
	var result, line string
	charCount := 0

	for _, char := range log {
		line += string(char)
		charCount++

		if charCount >= o.Width {
			if result != "" {
				result += "\n"
			}
			result += line
			line = ""
			charCount = 0
		}
	}

	if line != "" {
		if result != "" {
			result += "\n"
		}
		result += line
	}
	return result
}

type renderedOutput struct {
	plainErrorReceived string
	plainLogs          string
	plainJSON          string
	plainOutput        string

	error error

	styledError *strings.Builder
	styledLogs  *strings.Builder
	styledJSON  string
}

func (r *renderedOutput) highlighted() string {
	return ""
}

func (o *Output) renderedOutput(in *pbsubstreamsrpc.AnyModuleOutput, withStyle bool) (out *renderedOutput) {
	out = &renderedOutput{styledError: &strings.Builder{}, styledLogs: &strings.Builder{}}
	if in == nil {
		return out
	}
	dynamic.SetDefaultBytesRepresentation(o.bytesRepresentation)

	if o.errReceived != nil {
		out.styledError.WriteString(o.Styles.Output.ErrorLine.Render(o.errReceived.Error()))
	}
	if o.logsEnabled {
		if debugInfo := in.DebugInfo(); debugInfo != nil {
			var plainLogs []string
			for _, log := range debugInfo.Logs {
				plainLogs = append(plainLogs, fmt.Sprintf("log: %s", log))
				if withStyle {
					out.styledLogs.WriteString(o.Styles.Output.LogLabel.Render("log: "))
					out.styledLogs.WriteString(o.Styles.Output.LogLine.Render(o.wrapLogs(log)))
					out.styledLogs.WriteString("\n")
				}
			}
			if withStyle && len(debugInfo.Logs) != 0 {
				out.styledLogs.WriteString("\n")
			}
			out.plainLogs = strings.Join(plainLogs, "\n")
		}
	} else {
		out.plainLogs = ""
	}

	if in.IsMap() && !in.IsEmpty() {
		msgDesc := o.msgDescs[in.Name()]
		plain, err := o.decodeDynamicMessage(msgDesc, in.MapOutput.MapOutput)
		if err != nil {
			out.error = err
		}
		out.plainJSON = plain
		if withStyle {
			out.styledJSON = highlightJSON(plain)
		}
	}
	if in.IsStore() {
		if !in.IsEmpty() {
			msgDesc := o.msgDescs[in.Name()]
			// TODO: implement a store deltas decoder separate from JSON and styled one.
			out.plainOutput = o.decodeDynamicStoreDeltas(in.StoreOutput.DebugStoreDeltas, msgDesc)
		} else {
			out.plainOutput = "No deltas"
		}
	}
	return
}

func (o *Output) renderPayload(in *renderedOutput) string {
	out := &strings.Builder{}
	if in.error != nil {
		out.WriteString(o.Styles.Output.ErrorLine.Render(in.error.Error()))
		out.WriteString("\n")
	}
	if o.errReceived != nil {
		out.WriteString(in.styledError.String())
		out.WriteString("\n")
	}
	out.WriteString(in.styledLogs.String())
	out.WriteString(in.styledJSON)
	out.WriteString(in.plainOutput)
	return out.String()
}

func (o *Output) decodeDynamicMessage(msgDesc *manifest.ModuleDescriptor, anyin *anypb.Any) (string, error) {
	if msgDesc.MessageDescriptor == nil {
		return "", fmt.Errorf("no message descriptor for %s", anyin.MessageName())
		//return Styles.ErrorLine.Render(fmt.Sprintf("Unknown type: %s\n", anyin.MessageName()))
	}
	in := anyin.GetValue()
	dynMsg := o.messageFactory.NewDynamicMessage(msgDesc.MessageDescriptor)
	if err := dynMsg.Unmarshal(in); err != nil {
		return "", fmt.Errorf("failed unmarshalling message into %s: %s\n%s", msgDesc.ProtoMessageType,
			err.Error(),
			decodeAsString(in),
		)
		//return Styles.ErrorLine.Render(
		//	fmt.Sprintf("Failed unmarshalling message into %s: %s\n%s",
		//		msgDesc.ProtoMessageType,
		//		err.Error(),
		//		decodeAsString(in),
		//	),
		//)
	}

	cnt, err := dynMsg.MarshalJSONPB(&jsonpb.Marshaler{Indent: "  ", EmitDefaults: true})
	if err != nil {
		return "", fmt.Errorf("failed marshalling into JSON: %s\nString representation: %s", err.Error(), decodeAsString(in))
		//return Styles.ErrorLine.Render(
		//	fmt.Sprintf("Failed marshalling into JSON: %s\nString representation: %s",
		//		err.Error(),
		//		decodeAsString(in),
		//	),
		//)
	}

	return string(cnt), nil
}

func highlightJSON(in string) string {
	out := &strings.Builder{}
	profile := "friendly"
	if lipgloss.HasDarkBackground() {
		profile = "dracula"
	}
	if err := quick.Highlight(out, in, "json", "terminal256", profile); err != nil {
		return in
	}
	return out.String()
}

func (o *Output) decodeDynamicStoreDeltas(deltas []*pbsubstreamsrpc.StoreDelta, msgDesc *manifest.ModuleDescriptor) string {
	out := &strings.Builder{}
	for _, delta := range deltas {
		out.WriteString(fmt.Sprintf("%s (%d) KEY: %q\n", delta.Operation, delta.Ordinal, delta.Key))
		out.WriteString(o.decodeDelta(delta.OldValue, msgDesc, "OLD"))
		out.WriteString(o.decodeDelta(delta.NewValue, msgDesc, "NEW"))

		out.WriteString("\n")
	}
	return out.String()
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

func applyKeywordSearch(content, query string) (string, int, []int) {
	query = strings.TrimSpace(query)
	if query == "" {
		return content, 0, nil
	}

	var positions []int
	lines := strings.Split(content, "\n")
	newLines := make([]string, len(lines))
	var totalCount int
	for lineNo, line := range lines {
		count := strings.Count(line, query)
		totalCount += count
		if count != 0 {
			newLines[lineNo] = strings.ReplaceAll(line, query, termenv.String(query).Reverse().String())
			positions = append(positions, lineNo)
		} else {
			newLines[lineNo] = line
		}
	}
	return strings.Join(newLines, "\n"), totalCount, positions
}

func applyJQSearch(content, query string) (string, int, []int) {
	if len(content) == 0 {
		return "", 0, nil
	}
	var positions []int

	var decoded interface{}
	err := json.Unmarshal([]byte(content), &decoded)
	if err != nil {
		return fmt.Sprintf("error unmarshalling json from protobuf representation: %s", err), 0, nil
	}

	jqQuery, err := gojq.Parse(query)
	if err != nil {
		return fmt.Sprintf("error parsing jq expression: %s", err), 0, nil
	}

	code, err := gojq.Compile(jqQuery)
	if err != nil {
		return fmt.Sprintf("error compiling jq expression: %s", err), 0, nil
	}

	var lines []string
	var count int
	it := code.Run(decoded)
	for {
		el, ok := it.Next()
		if !ok {
			break
		}
		count++

		if err, ok := el.(error); ok {
			lines = append(lines, "error: "+err.Error())
			continue
		}
		//log.Printf("MAMA %p %T %v %v", el, el, el, ok)

		cnt, err := json.MarshalIndent(el, "", "  ")
		if err != nil {
			return fmt.Sprintf("error marshalling json: %s", err), 0, nil
		}

		lines = append(lines, string(cnt))
	}

	return strings.Join(lines, "\n"), count, positions
}
