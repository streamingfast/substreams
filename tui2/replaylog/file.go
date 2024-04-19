package replaylog

import (
	"fmt"
	"io"
	"log"
	"os"

	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"

	"github.com/streamingfast/dbin"

	tea "github.com/charmbracelet/bubbletea"
	"google.golang.org/protobuf/proto"
	anypb "google.golang.org/protobuf/types/known/anypb"

	"github.com/streamingfast/substreams/tui2/stream"
)

type File struct {
	writer *dbin.Writer
	path   string
}

const ReplayContentType = "rpl-v1"
const ReplayFilename = "replay.log"

type Option func(*File)

func WithPath(path string) Option {
	return func(f *File) {
		f.path = path
	}
}

func New(opts ...Option) *File {
	f := &File{
		path: ReplayFilename,
	}

	for _, opt := range opts {
		opt(f)
	}

	return f
}

func (f *File) IsWriting() bool {
	return f.writer != nil
}

func (f *File) OpenForWriting() error {
	fl, err := os.OpenFile(ReplayFilename, os.O_WRONLY|os.O_CREATE, 0640)
	if err != nil {
		return fmt.Errorf("open replay file for writing: %w", err)
	}
	f.writer = dbin.NewWriter(fl)
	if err := f.writer.WriteHeader(ReplayContentType); err != nil {
		return fmt.Errorf("write replay header: %w", err)
	}
	return nil
}

func (f *File) ReadReplay() (out stream.ReplayBundle, err error) {
	fl, err := os.OpenFile(ReplayFilename, os.O_RDONLY, 0640)
	if err != nil {
		return nil, fmt.Errorf("read replay file: %w", err)
	}
	defer fl.Close()

	reader := dbin.NewReader(fl)
	header, err := reader.ReadHeader()
	if err != nil {
		return nil, fmt.Errorf("reading replay log header: %w", err)
	}
	if header.ContentType != ReplayContentType {
		return nil, fmt.Errorf("invalid replay file content type %q", header.ContentType)
	}
	for {
		anyBytes, err := reader.ReadMessage()
		if err != nil {
			if err == io.EOF {
				break
			}
			if err == io.ErrUnexpectedEOF {
				log.Println("Unexpected EOF reading replay file, ignoring it.")
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
	out = append(out, stream.ReplayedMsg)

	return
}

func mapTypeToUpdateMsg(in any) any {
	switch m := in.(type) {
	case *pbsubstreamsrpc.Request,
		*pbsubstreamsrpc.BlockScopedData,
		*pbsubstreamsrpc.ModulesProgress,
		*pbsubstreamsrpc.InitialSnapshotData,
		*pbsubstreamsrpc.InitialSnapshotComplete,
		*pbsubstreamsrpc.SessionInit:
		return m
	}
	panic("unsupported payload")
}

func (f *File) Push(msg tea.Msg) error {
	if f.writer == nil {
		return nil
	}

	switch msg.(type) {
	case *pbsubstreamsrpc.Request,
		*pbsubstreamsrpc.BlockScopedData,
		*pbsubstreamsrpc.ModulesProgress,
		*pbsubstreamsrpc.InitialSnapshotData,
		*pbsubstreamsrpc.InitialSnapshotComplete,
		*pbsubstreamsrpc.SessionInit:

		anyMsg, err := anypb.New(msg.(proto.Message))
		if err != nil {
			return fmt.Errorf("encoding any: %w", err)
		}
		_ = anyMsg
		cnt, err := proto.Marshal(anyMsg)
		if err != nil {
			return fmt.Errorf("proto marshal replay msg: %w", err)
		}
		if err := f.writer.WriteMessage(cnt); err != nil {
			return fmt.Errorf("write replay message: %w", err)
		}
	}
	return nil
}

func (f *File) Close() error {
	if f.writer != nil {
		return f.writer.Close()
	}
	return nil
}
