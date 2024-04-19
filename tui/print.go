package tui

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/dynamic"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/tidwall/pretty"
	"google.golang.org/protobuf/types/known/anypb"
)

func (ui *TUI) decoratedBlockScopedData(
	output *pbsubstreamsrpc.MapModuleOutput,
	debugMapOutputs []*pbsubstreamsrpc.MapModuleOutput,
	debugStoreOutputs []*pbsubstreamsrpc.StoreModuleOutput,
	clock *pbsubstreams.Clock,
) error {
	var s []string

	for _, out := range append([]*pbsubstreamsrpc.MapModuleOutput{output}, debugMapOutputs...) {
		if _, ok := ui.msgTypes[out.Name]; !ok {
			continue
		}
		if out.DebugInfo != nil {
			for _, log := range out.DebugInfo.Logs {
				s = append(s, fmt.Sprintf("%s: log: %s\n", out.Name, log))
			}
		}

		if len(out.MapOutput.Value) != 0 {
			msgDesc := ui.msgDescs[out.Name]
			msgType := ui.msgTypes[out.Name]
			cnt := ui.decodeDynamicMessage(msgType, msgDesc, clock.Number, out.Name, out.MapOutput)
			cnt = ui.prettyFormat(cnt, true)
			if out.DebugInfo != nil && out.DebugInfo.Cached {
				s = append(s, cachedValues(out.Name))
			}
			s = append(s, string(cnt))
		}
	}

	for _, out := range debugStoreOutputs {
		if _, ok := ui.msgTypes[out.Name]; !ok {
			continue
		}
		for _, log := range out.DebugInfo.Logs {
			s = append(s, fmt.Sprintf("%s: log: %s\n", out.Name, log))
		}

		if len(out.DebugStoreDeltas) != 0 {
			if out.DebugInfo != nil && out.DebugInfo.Cached {
				s = append(s, cachedValues(out.Name))
			}
			s = append(s, ui.renderDecoratedDeltas(out.Name, clock.Number, out.DebugStoreDeltas, false)...)
		}
	}

	if len(s) != 0 {
		fmt.Println(strings.Join(s, ""))
	}
	return nil
}

func cachedValues(name string) string {
	return fmt.Sprintf("Cached value(s) for %s\n", name)
}

func (ui *TUI) renderDecoratedDeltas(modName string, blockNum uint64, deltas []*pbsubstreamsrpc.StoreDelta, initialSnapshot bool) (s []string) {
	msgDesc := ui.msgDescs[modName]
	msgType := ui.msgTypes[modName]
	if initialSnapshot {
		s = append(s, fmt.Sprintf("%s: initial store snapshot:\n", modName))
	} else {
		s = append(s, fmt.Sprintf("%s: store deltas:\n", modName))
	}
	for _, delta := range deltas {
		keyStr, _ := json.Marshal(delta.Key)
		s = append(s, fmt.Sprintf("  %s (%d) KEY: %s\n", delta.Operation.String(), delta.Ordinal, ui.prettyFormat(keyStr, false)))

		if len(delta.NewValue) == 0 {
			s = append(s, "    NEW: (none)\n")
		} else {
			new := ui.decodeDynamicStoreDeltas(msgType, msgDesc, blockNum, modName, delta.NewValue)
			s = append(s, fmt.Sprintf("    NEW: %s\n", indent(ui.prettyFormat(new, false))))
		}
	}
	return
}

func (ui *TUI) printJSONBlockDeltas(modName string, blockNum uint64, deltas []*pbsubstreamsrpc.StoreDelta) error {
	wrap := DeltasWrap{
		Module:   modName,
		BlockNum: blockNum,
	}
	msgDesc := ui.msgDescs[modName]
	msgType := ui.msgTypes[modName]
	for _, delta := range deltas {
		subwrap := DeltaWrap{
			Operation: delta.Operation.String(),
			Ordinal:   delta.Ordinal,
			Key:       delta.Key,
		}
		if len(delta.NewValue) != 0 {
			new := ui.decodeDynamicStoreDeltas(msgType, msgDesc, 0, modName, delta.NewValue)
			subwrap.NewValue = json.RawMessage(new)
		}
		wrap.Deltas = append(wrap.Deltas, subwrap)
	}
	cnt, err := json.Marshal(wrap)
	if err != nil {
		return fmt.Errorf("marshal wrap: %w", err)
	}
	fmt.Println(string(ui.prettyFormat(cnt, false)))
	return nil
}

func indent(in []byte) []byte {
	return bytes.Replace(in, []byte("\n"), []byte("\n    "), -1)
}

func (ui *TUI) jsonBlockScopedData(
	output *pbsubstreamsrpc.MapModuleOutput,
	debugMapOutputs []*pbsubstreamsrpc.MapModuleOutput,
	debugStoreOutputs []*pbsubstreamsrpc.StoreModuleOutput,
	clock *pbsubstreams.Clock,
) error {

	for _, out := range append([]*pbsubstreamsrpc.MapModuleOutput{output}, debugMapOutputs...) {
		if _, ok := ui.msgTypes[out.Name]; !ok {
			continue
		}

		if len(out.MapOutput.Value) != 0 {
			msgDesc := ui.msgDescs[out.Name]
			msgType := ui.msgTypes[out.Name]
			cnt := ui.decodeDynamicMessage(msgType, msgDesc, clock.Number, out.Name, out.MapOutput)
			cnt = ui.prettyFormat(cnt, true)
			if out.DebugInfo != nil && out.DebugInfo.Cached {
				fmt.Println(cachedValues(out.Name))
			}
			fmt.Println(string(cnt))
		}
	}

	for _, out := range debugStoreOutputs {
		if _, ok := ui.msgTypes[out.Name]; !ok {
			continue
		}
		if len(out.DebugStoreDeltas) != 0 {
			if out.DebugInfo != nil && out.DebugInfo.Cached {
				fmt.Println(cachedValues(out.Name))
			}
			if err := ui.printJSONBlockDeltas(out.Name, clock.Number, out.DebugStoreDeltas); err != nil {
				return fmt.Errorf("print json deltas: %w", err)
			}
		}
	}
	return nil
}

func (ui *TUI) decoratedSnapshotData(output *pbsubstreamsrpc.InitialSnapshotData) error {
	var s []string
	if output != nil && len(output.Deltas) != 0 {
		s = append(s, ui.renderDecoratedDeltas(output.ModuleName, 0, output.Deltas, true)...)
	}
	if len(s) != 0 {
		fmt.Println(strings.Join(s, ""))
	}
	return nil
}

func (ui *TUI) jsonSnapshotData(output *pbsubstreamsrpc.InitialSnapshotData) error {
	if len(output.Deltas) == 0 {
		return nil
	}

	modName := output.ModuleName
	msgDesc := ui.msgDescs[modName]
	msgType := ui.msgTypes[modName]
	length := len(output.Deltas)
	for idx, delta := range output.Deltas {
		wrap := SnapshotDeltaWrap{
			Module:   modName,
			Progress: fmt.Sprintf("%.2f %%", float64(int(output.SentKeys)-length+idx+1)/float64(output.TotalKeys)*100),
		}
		subwrap := DeltaWrap{
			Operation: delta.Operation.String(),
			Ordinal:   delta.Ordinal,
			Key:       delta.Key,
		}
		if len(delta.NewValue) != 0 {
			new := ui.decodeDynamicStoreDeltas(msgType, msgDesc, 0, modName, delta.NewValue)
			subwrap.NewValue = json.RawMessage(new)
		}
		wrap.Delta = subwrap
		cnt, err := json.Marshal(wrap)
		if err != nil {
			return fmt.Errorf("marshal wrap: %w", err)
		}
		fmt.Println(string(ui.prettyFormat(cnt, false)))
	}
	return nil
}

func (ui *TUI) decodeDynamicMessage(msgType string, msgDesc *desc.MessageDescriptor, blockNum uint64, modName string, anyin *anypb.Any) []byte {
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

	cnt, err := msgDescToJSON(msgType, blockNum, modName, dynMsg, true)
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

func (ui *TUI) decodeDynamicStoreDeltas(msgType string, msgDesc *desc.MessageDescriptor, blockNum uint64, modName string, in []byte) []byte {
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
		cnt, err := msgDescToJSON(msgType, blockNum, modName, dynMsg, false)
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

func (ui *TUI) prettyFormat(cnt []byte, isMapOutput bool) []byte {
	if ui.prettyPrintOutput {
		if isMapOutput {
			cnt = pretty.Pretty(cnt)
		} else {
			cnt = bytes.TrimRight(pretty.Pretty(cnt), "\n")
		}
	}
	if ui.isTerminal {
		cnt = pretty.Color(cnt, pretty.TerminalStyle)
	}
	return cnt
}

func msgDescToJSON(msgType string, blockNum uint64, mod string, dynMsg *dynamic.Message, wrap bool) (cnt []byte, err error) {
	cnt, err = dynMsg.MarshalJSON()
	if err != nil {
		return
	}

	if wrap {
		// FIXME: don't module wrap when we're in terminal mode and decorated output?
		cnt, err = json.Marshal(ModuleWrap{
			Module:   mod,
			BlockNum: blockNum,
			Type:     msgType,
			Data:     cnt,
		})
		if err != nil {
			return
		}
	}

	return
}

type DeltasWrap struct {
	Module   string      `json:"@module"`
	BlockNum uint64      `json:"@block,omitempty"`
	Deltas   []DeltaWrap `json:"@deltas"`
}

type SnapshotDeltaWrap struct {
	Module   string    `json:"@module"`
	Progress string    `json:"@progress"`
	Delta    DeltaWrap `json:"@delta"`
}

type DeltaWrap struct {
	Operation string          `json:"op"`
	Ordinal   uint64          `json:"ordinal"`
	Key       string          `json:"key"`
	OldValue  json.RawMessage `json:"old"`
	NewValue  json.RawMessage `json:"new"`
}

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

func decodeAsString(in []byte) []byte { return []byte(fmt.Sprintf("%q", string(in))) }
func decodeAsHex(in []byte) string    { return "(hex) " + hex.EncodeToString(in) }

func printClock(block *pbsubstreamsrpc.BlockScopedData) {
	fmt.Printf("----------- BLOCK #%s (%s) ---------------\n", humanize.Comma(int64(block.Clock.Number)), block.Clock.Id)
}

func printUndo(lastGoodClock *pbsubstreams.BlockRef, cursor string) {
	fmt.Printf("----------- BLOCK UNDO UP TO #%s (0x%s) ---------------\n", humanize.Comma(int64(lastGoodClock.Number)), lastGoodClock.Id)
	fmt.Printf("\nNext cursor: %s\n", cursor)
}
func printUndoJSON(lastGoodClock *pbsubstreams.BlockRef, cursor string) {
	fmt.Printf(formatUndoJSON(lastGoodClock, cursor) + "\n")
}

func formatUndoJSON(lastGoodClock *pbsubstreams.BlockRef, cursor string) string {
	return fmt.Sprintf("{\"undo_until\":{\"num\":%d,\"id\":\"%s\",\"next_cursor\":\"%s\"}}", lastGoodClock.Number, lastGoodClock.Id, cursor)
}
