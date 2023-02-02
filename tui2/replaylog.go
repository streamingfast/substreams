package tui2

import (
	"fmt"
	"io"
	"os"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"

	"github.com/streamingfast/dbin"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/streamingfast/substreams/tui2/stream"
	"google.golang.org/protobuf/proto"
	anypb "google.golang.org/protobuf/types/known/anypb"
)

type ReplayBundle []any

type ReplayLog struct {
	fl *os.File
}

func NewReplayLog() (*ReplayLog, error) {
	fl, err := os.OpenFile("replay.log", os.O_RDWR|os.O_CREATE, 0640)
	if err != nil {
		return nil, fmt.Errorf("open replay file: %w", err)
	}

	return &ReplayLog{
		fl: fl,
	}, nil
}

func (v *ReplayLog) Fetch() (out ReplayBundle, err error) {
	reader := dbin.NewReader(v.fl)
	header, ver, err := reader.ReadHeader()
	if err != nil {
		return nil, fmt.Errorf("reading replay log header: %w", err)
	}
	if header != "rpl" || ver != 0 {
		return nil, fmt.Errorf("invalid replay file format/version: %w", err)
	}
	for {
		anyBytes, err := reader.ReadMessage()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("reading replay file: %w", err)
		}

		newAny := &anypb.Any{}
		if err := proto.Unmarshal(anyBytes, newAny); err != nil {
			return nil, fmt.Errorf("reading any from replay file: %w", err)
		}

		newVal, err := anypb.UnmarshalNew(newAny, proto.UnmarshalOptions{})
		if err != nil {
			return nil, fmt.Errorf("unmarshal any from replay file: %w", err)
		}

		out = append(out, mapTypeToUpdateMsg(newVal))
	}
	v.fl.Truncate(0)
	out = append(out, stream.ReplayedMsg)
	return
}
func mapTypeToUpdateMsg(in any) any {
	switch m := in.(type) {
	case *pbsubstreams.Request:
		return stream.SetRequestMsg(m)
	case *pbsubstreams.BlockScopedData:
		return stream.ResponseDataMsg(m)
	case *pbsubstreams.ModulesProgress:
		return stream.ResponseProgressMsg(m)
	case *pbsubstreams.InitialSnapshotData:
		return stream.ResponseInitialSnapshotDataMsg(m)
	case *pbsubstreams.InitialSnapshotComplete:
		return stream.ResponseInitialSnapshotCompleteMsg(m)
	}
	panic("wohasd")
}

func (v *ReplayLog) Push(msg tea.Msg) error {
	return nil
	switch msg.(type) {
	case stream.SetRequestMsg,
		stream.ResponseProgressMsg,
		stream.ResponseInitialSnapshotDataMsg,
		stream.ResponseInitialSnapshotCompleteMsg,
		stream.ResponseDataMsg:
		anyMsg, err := anypb.New(msg.(proto.Message))
		if err != nil {
			return fmt.Errorf("encoding any: %w", err)
		}
		_ = anyMsg
		// TODO: write to dbin file.

	}
	return nil
}

func (v *ReplayLog) Close() error {
	return v.fl.Close()
}
