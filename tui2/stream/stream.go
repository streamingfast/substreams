package stream

import (
	"context"
	"fmt"
	"io"
	"log"

	tea "github.com/charmbracelet/bubbletea"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"google.golang.org/grpc"
)

type Msg int

const (
	ConnectingMsg Msg = iota
	ConnectedMsg
	InterruptStreamMsg
	EndOfStreamMsg
)

type SetRequestMsg *pbsubstreams.Request
type StreamErrorMsg error
type ResponseDataMsg *pbsubstreams.BlockScopedData
type ResponseProgressMsg *pbsubstreams.ModulesProgress
type ResponseInitialSnapshotDataMsg *pbsubstreams.InitialSnapshotData
type ResponseInitialSnapshotCompleteMsg *pbsubstreams.InitialSnapshotComplete
type ResponseUnknownMsg string

type Stream struct {
	req            *pbsubstreams.Request
	client         pbsubstreams.StreamClient
	callOpts       []grpc.CallOption
	targetEndBlock uint64

	ctx           context.Context
	cancelContext func()
	conn          pbsubstreams.Stream_BlocksClient

	err error
}

func New(req *pbsubstreams.Request, client pbsubstreams.StreamClient, callOpts []grpc.CallOption) *Stream {
	return &Stream{
		req:            req,
		targetEndBlock: req.StopBlockNum,
		client:         client,
		callOpts:       callOpts,
	}
}

func (s *Stream) StreamColor() string {
	if s.err != nil && s.err != io.EOF {
		return "9"
	}
	if s.cancelContext != nil || s.err == io.EOF {
		return "2"
	}
	return "3"
}

func (s *Stream) LinearHandoffBlock() uint64 {
	return uint64(s.req.StartBlockNum)
}

func (s *Stream) Init() tea.Cmd {
	return tea.Sequence(
		func() tea.Msg {
			return SetRequestMsg(s.req)
		},
		func() tea.Msg {
			return ConnectingMsg
		},
		s.StartStream,
	)
}

func (s *Stream) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case StreamErrorMsg:
		s.err = msg
	case ResponseDataMsg,
		ResponseProgressMsg,
		ResponseInitialSnapshotDataMsg,
		ResponseInitialSnapshotCompleteMsg,
		ResponseUnknownMsg:
		return s.readNextMessage
	case Msg:
		switch msg {
		case ConnectedMsg:
			return s.readNextMessage
		case InterruptStreamMsg:
			if s.cancelContext != nil {
				s.cancelContext()
				s.cancelContext = nil
			}
		case EndOfStreamMsg:
			s.err = io.EOF
		}
	}
	return nil
}

func (s *Stream) StartStream() tea.Msg {
	streamCtx, cancel := context.WithCancel(context.Background())
	s.ctx = streamCtx
	s.cancelContext = cancel

	cli, err := s.client.Blocks(streamCtx, s.req, s.callOpts...)
	if err != nil && streamCtx.Err() != context.Canceled {
		return StreamErrorMsg(fmt.Errorf("call sf.substreams.v1.Stream/Blocks: %w", err))
	}

	s.conn = cli

	return ConnectedMsg
}

func (s *Stream) readNextMessage() tea.Msg {
	if s.err != nil {
		return nil
	}

	resp, err := s.conn.Recv()
	if err != nil {
		if err == io.EOF {
			s.err = io.EOF
			return EndOfStreamMsg
		}
		return StreamErrorMsg(fmt.Errorf("read next message: %w", err))
	}
	return s.routeNextMessage(resp)
}

func (s *Stream) routeNextMessage(resp *pbsubstreams.Response) tea.Msg {
	switch m := resp.Message.(type) {
	case *pbsubstreams.Response_Data:
		return ResponseDataMsg(m.Data)
	case *pbsubstreams.Response_Progress:
		log.Printf("Progress response: %T %v", resp, resp)
		return ResponseProgressMsg(m.Progress)
	case *pbsubstreams.Response_DebugSnapshotData:
		return ResponseInitialSnapshotDataMsg(m.DebugSnapshotData)
	case *pbsubstreams.Response_DebugSnapshotComplete:
		return ResponseInitialSnapshotCompleteMsg(m.DebugSnapshotComplete)
	}
	return ResponseUnknownMsg(fmt.Sprintf("%T", resp.Message))
}
