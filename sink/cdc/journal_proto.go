package cdc

import (
	"bytes"
	"encoding/json"
	"fmt"

	"google.golang.org/protobuf/types/known/timestamppb"

	pbcdc "github.com/streamingfast/substreams/pb/sf/substreams/sink/cdc/v1"
)

// toProtoEvent keeps what a compensation needs, the cursor and final flag are not persisted since both
// are set again when the compensation is built.
func toProtoEvent(e *Event) (*pbcdc.JournalEvent, error) {
	out := &pbcdc.JournalEvent{
		Id:      e.ID,
		Op:      string(e.Op),
		Key:     e.Key,
		Partial: e.Partial,
		Module:  e.Source.Module,
		Table:   e.Source.Table,
		Ordinal: e.Source.Ordinal,
	}
	if !e.Source.Block.Timestamp.IsZero() {
		out.BlockTimestamp = timestamppb.New(e.Source.Block.Timestamp)
	}

	if e.After != nil {
		after, err := json.Marshal(e.After)
		if err != nil {
			return nil, fmt.Errorf("encode after of event %q: %w", e.ID, err)
		}
		out.AfterJson = after
	}
	return out, nil
}

func fromProtoEvent(block *pbcdc.JournalBlock, e *pbcdc.JournalEvent) (Event, error) {
	out := Event{
		ID:      e.Id,
		Op:      Op(e.Op),
		Key:     e.Key,
		Partial: e.Partial,
		Source: Source{
			Module:  e.Module,
			Table:   e.Table,
			Ordinal: e.Ordinal,
			Block:   Block{Number: block.Number, ID: block.Id},
		},
	}
	if e.BlockTimestamp != nil {
		out.Source.Block.Timestamp = e.BlockTimestamp.AsTime()
	}

	if len(e.AfterJson) > 0 {
		dec := json.NewDecoder(bytes.NewReader(e.AfterJson))
		dec.UseNumber()
		if err := dec.Decode(&out.After); err != nil {
			return Event{}, fmt.Errorf("decode after of event %q: %w", e.Id, err)
		}
	}
	return out, nil
}
