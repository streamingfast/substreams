package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/streamingfast/dgrpc/server"
	"github.com/streamingfast/dgrpc/server/standard"
	"google.golang.org/grpc"
	"google.golang.org/grpc/encoding"
)

var dynCmd = &cobra.Command{
	Use:          "dyngrpc <package>",
	Short:        "Test a dynamic gRPC server",
	RunE:         runDyn,
	SilenceUsage: true,
}

func init() {
	rootCmd.AddCommand(dynCmd)
}

func runDyn(cmd *cobra.Command, args []string) error {
	encoding.RegisterCodec(passthroughCodec{})
	s := standard.NewServer(server.NewOptions())
	srv := s.GrpcServer()
	genericService := &GenericService{}
	var v interface{}

	// TODO: generate this ServiceDesc struct from the
	// loaded .proto files we'd receive (from within the Substreams `spkg` ?)
	// or reuse the dynamic protobuf loading we use in `substreams`.
	srv.RegisterService(&grpc.ServiceDesc{
		ServiceName: "sf.mycustomer.v1.Eth",
		HandlerType: genericService,
		Methods:     []grpc.MethodDesc{},
		Streams: []grpc.StreamDesc{
			{
				StreamName:    "Transfers",
				Handler:       genericService.New("Transfers").handle,
				ServerStreams: true,
			},
		},
		Metadata: "sf/mycustomer/v1/eth.proto",
	}, v)

	fmt.Println("Listening on :7878")
	s.Launch(":7878")

	return nil
}

type GenericService struct{}

type GenericHandler struct {
	streamName string
}

func (s *GenericService) New(streamName string) *GenericHandler {
	return &GenericHandler{streamName: streamName}
}
func (h *GenericHandler) handle(server interface{}, stream grpc.ServerStream) error {
	m := NewPassthroughBytes()
	if err := stream.RecvMsg(m); err != nil {
		return err
	}

	// TODO: invoke the WASM machine, using `h.streamName`,
	// take the return value
	// and pipe it back to the `stream.SendMsg()` as Bytes, as a PassthroughBytes
	// object.

	stream.SendMsg(m)
	return nil
}

// Codec

type passthroughCodec struct{}

func (passthroughCodec) Marshal(v interface{}) ([]byte, error) {
	return v.(*passthroughBytes).Bytes, nil
}

func (passthroughCodec) Unmarshal(data []byte, v interface{}) error {
	el := v.(*passthroughBytes)
	el.Bytes = data
	return nil
}

func (passthroughCodec) Name() string { return "proto" }

// Passing bytes around

type passthroughBytes struct {
	Bytes []byte
}

func NewPassthroughBytes() *passthroughBytes {
	return &passthroughBytes{}
}
func (b *passthroughBytes) Set(in []byte) {
	b.Bytes = in
}
