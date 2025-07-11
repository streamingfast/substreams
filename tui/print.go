package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/golang/protobuf/jsonpb"
	protoV1 "github.com/golang/protobuf/proto"
	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/tidwall/pretty"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"
)

func (ui *TUI) decoratedBlockScopedData(
	output *pbsubstreamsrpc.MapModuleOutput,
	debugMapOutputs []*pbsubstreamsrpc.MapModuleOutput,
	debugStoreOutputs []*pbsubstreamsrpc.StoreModuleOutput,
	clock *pbsubstreams.Clock,
) error {
	var s []string
	for _, out := range append([]*pbsubstreamsrpc.MapModuleOutput{output}, debugMapOutputs...) {
		if !ui.decoder.HasMessageType(out.Name) {
			continue
		}
		if out.DebugInfo != nil {
			for _, log := range out.DebugInfo.Logs {
				s = append(s, fmt.Sprintf("%s: log: %s\n", out.Name, log))
			}
		}

		if len(out.MapOutput.Value) != 0 {
			msgDesc := ui.decoder.GetMessageDescriptor(out.Name)
			msgType := ui.decoder.GetMessageType(out.Name)
			cnt := ui.decoder.DecodeDynamicMessage(msgType, msgDesc, clock.Number, out.Name, out.MapOutput)
			cnt = ui.prettyFormat(cnt, true)
			if out.DebugInfo != nil && out.DebugInfo.Cached {
				s = append(s, cachedValues(out.Name))
			}
			s = append(s, string(cnt))
		}
	}

	for _, out := range debugStoreOutputs {
		if !ui.decoder.HasMessageType(out.Name) {
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
	msgDesc := ui.decoder.GetMessageDescriptor(modName)
	msgType := ui.decoder.GetMessageType(modName)
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
			new := ui.decoder.DecodeDynamicStoreDeltas(msgType, msgDesc, blockNum, modName, delta.NewValue)
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
	msgDesc := ui.decoder.GetMessageDescriptor(modName)
	msgType := ui.decoder.GetMessageType(modName)
	for _, delta := range deltas {
		subwrap := DeltaWrap{
			Operation: delta.Operation.String(),
			Ordinal:   delta.Ordinal,
			Key:       delta.Key,
		}
		if len(delta.NewValue) != 0 {
			new := ui.decoder.DecodeDynamicStoreDeltas(msgType, msgDesc, 0, modName, delta.NewValue)
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
		if !ui.decoder.HasMessageType(out.Name) {
			continue
		}

		if len(out.MapOutput.Value) != 0 {
			msgDesc := ui.decoder.GetMessageDescriptor(out.Name)
			msgType := ui.decoder.GetMessageType(out.Name)
			cnt := ui.decoder.DecodeDynamicMessage(msgType, msgDesc, clock.Number, out.Name, out.MapOutput)
			cnt = ui.prettyFormat(cnt, true)
			if out.DebugInfo != nil && out.DebugInfo.Cached {
				fmt.Println(cachedValues(out.Name))
			}
			fmt.Println(string(cnt))
		}
	}

	for _, out := range debugStoreOutputs {
		if !ui.decoder.HasMessageType(out.Name) {
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
	msgDesc := ui.decoder.GetMessageDescriptor(modName)
	msgType := ui.decoder.GetMessageType(modName)
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
			new := ui.decoder.DecodeDynamicStoreDeltas(msgType, msgDesc, 0, modName, delta.NewValue)
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

var _ jsonpb.AnyResolver = (*wellKnowAnyResolver)(nil)

type wellKnowAnyResolver struct {
	registry *protoregistry.Files
}

func (w *wellKnowAnyResolver) Resolve(typeURL string) (protoV1.Message, error) {
	sanitizedTypeURL := strings.TrimPrefix(typeURL, "type.googleapis.com/")

	desc, err := w.registry.FindDescriptorByName(protoreflect.FullName(sanitizedTypeURL))
	if err != nil {
		return nil, fmt.Errorf("find descriptor by name: %w", err)
	}

	msgDesc, ok := desc.(protoreflect.MessageDescriptor)
	if !ok {
		return nil, fmt.Errorf("descriptor is not found or not of message type for typeURL %s", typeURL)
	}

	return protoV1.MessageV1(dynamicpb.NewMessage(msgDesc)), nil
}

func newWellKnowAnyResolver(pkg *pbsubstreams.Package) (*wellKnowAnyResolver, error) {
	files, err := pkg.GetProtoFilesRegistry()
	if err != nil {
		return nil, fmt.Errorf("get proto files registry: %w", err)
	}

	return &wellKnowAnyResolver{
		registry: files,
	}, nil
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

func decodeAsString(in []byte) []byte { return []byte(fmt.Sprintf("%q", string(in))) }

func printClock(block *pbsubstreamsrpc.BlockScopedData) {
	fmt.Printf("----------- BLOCK #%s (%s) age=%s ---------------\n", humanize.Comma(int64(block.Clock.Number)), block.Clock.Id, time.Since(block.Clock.Timestamp.AsTime()))
}

func printUndo(lastGoodClock *pbsubstreams.BlockRef, cursor string) {
	fmt.Printf("----------- BLOCK UNDO UP TO #%s (0x%s) ---------------\n", humanize.Comma(int64(lastGoodClock.Number)), lastGoodClock.Id)
	fmt.Printf("\nNext cursor: %s\n", cursor)
}
func printUndoJSON(lastGoodClock *pbsubstreams.BlockRef, cursor string) {
	fmt.Print(formatUndoJSON(lastGoodClock, cursor) + "\n")
}

func formatUndoJSON(lastGoodClock *pbsubstreams.BlockRef, cursor string) string {
	return fmt.Sprintf("{\"undo_until\":{\"num\":%d,\"id\":\"%s\",\"next_cursor\":\"%s\"}}", lastGoodClock.Number, lastGoodClock.Id, cursor)
}
