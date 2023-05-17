package stream

import (
	"context"
	"fmt"
	"io"
	"log"

	tea "github.com/charmbracelet/bubbletea"
	"google.golang.org/grpc"

	pbsubstreamsrpc "github.com/streamingfast/substreams/pb/sf/substreams/rpc/v2"
)

type Msg int

const (
	ConnectingMsg Msg = iota
	ConnectedMsg
	InterruptStreamMsg
	EndOfStreamMsg
	ReplayedMsg
)

type ReplayBundle []any

type StreamErrorMsg error
type ResponseUnknownMsg string

type Stream struct {
	ReplayBundle ReplayBundle

	req            *pbsubstreamsrpc.Request
	client         pbsubstreamsrpc.StreamClient
	callOpts       []grpc.CallOption
	targetEndBlock uint64

	ctx           context.Context
	cancelContext func()
	conn          pbsubstreamsrpc.Stream_BlocksClient

	err error
}

func New(req *pbsubstreamsrpc.Request, client pbsubstreamsrpc.StreamClient, callOpts []grpc.CallOption) *Stream {
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

func (s *Stream) TargetParallelProcessingBlock() uint64 {
	if s.req.ProductionMode {
		return s.req.StopBlockNum
	}
	return uint64(s.req.StartBlockNum)
}

func (s *Stream) Init() tea.Cmd {
	if s.ReplayBundle != nil {
		bundle := s.ReplayBundle
		s.ReplayBundle = nil
		return func() tea.Msg {
			return bundle
		}
	}
	return tea.Sequence(
		func() tea.Msg {
			return s.req
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
	case *pbsubstreamsrpc.BlockScopedData,
		*pbsubstreamsrpc.ModulesProgress,
		*pbsubstreamsrpc.InitialSnapshotData,
		*pbsubstreamsrpc.InitialSnapshotComplete,
		*pbsubstreamsrpc.SessionInit,
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
		return StreamErrorMsg(fmt.Errorf("call sf.substreams.rpc.v2.Stream/Blocks: %w", err))
	}

	s.conn = cli

	return ConnectedMsg
}

func (s *Stream) readNextMessage() tea.Msg {
	if s.err != nil {
		return nil
	}

	if s.conn != nil {
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
	return nil
}

func (s *Stream) routeNextMessage(resp *pbsubstreamsrpc.Response) tea.Msg {
	switch m := resp.Message.(type) {
	case *pbsubstreamsrpc.Response_BlockScopedData:
		return m.BlockScopedData
	case *pbsubstreamsrpc.Response_Progress:
		log.Printf("Progress response: %T %v", resp, resp)
		return m.Progress
	case *pbsubstreamsrpc.Response_DebugSnapshotData:
		return m.DebugSnapshotData
	case *pbsubstreamsrpc.Response_DebugSnapshotComplete:
		return m.DebugSnapshotComplete
	case *pbsubstreamsrpc.Response_Session:
		return m.Session
	}
	return ResponseUnknownMsg(fmt.Sprintf("%T", resp.Message))
}
