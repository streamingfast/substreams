package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/second-state/WasmEdge-go/wasmedge"
	bindgen "github.com/second-state/wasmedge-bindgen/host/go"
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
				Handler:       genericService.New("sf.mycustomer.v1.Eth.Transfers").handle,
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
	exportName string
}

func (s *GenericService) New(streamName string) *GenericHandler {
	exportName := strings.Replace(streamName, ".", "_", -1)
	exportName = strings.Replace(exportName, "/", "_", -1)
	exportName = strings.ToLower(exportName)
	// TODO: do validation that there are only letters and digits left
	return &GenericHandler{exportName: exportName}
}
func (h *GenericHandler) handle(server interface{}, stream grpc.ServerStream) error {
	m := NewPassthroughBytes()
	if err := stream.RecvMsg(m); err != nil {
		return err
	}

	// See: https://github.com/second-state/WasmEdge-go-examples/blob/master/wasmedge-bindgen/go_BindgenFuncs/bindgen_funcs.go
	wasmedge.SetLogErrorLevel()
	conf := wasmedge.NewConfigure(wasmedge.WASI)
	vm := wasmedge.NewVMWithConfig(conf)
	wasi := vm.GetImportModule(wasmedge.WASI)
	wasi.InitWasi(
		os.Args[1:],     // The args
		os.Environ(),    // The envs
		[]string{".:."}, // The mapping preopens
	)
	if err := vm.LoadWasmFile("./cmd/substreams/dyntest/eth_xfer.wasm"); err != nil {
		return fmt.Errorf("load wasm: %w", err)
	}
	if err := vm.Validate(); err != nil {
		return fmt.Errorf("validate: %w", err)
	}

	fmt.Println("PAPA")
	bg := bindgen.New(vm)
	bg.Instantiate()

	fmt.Println("MAMA")
	// TODO: invoke the WASM machine, using `h.streamName`,
	// take the return value
	// and pipe it back to the `stream.SendMsg()` as Bytes, as a PassthroughBytes
	// object.
	res, _, err := bg.Execute(h.exportName, m.Bytes)
	if err != nil {
		return fmt.Errorf("failed executing bindgen func %q: %w", h.exportName, err)
	}
	out := NewPassthroughBytes()
	out.Set(res[0].([]byte))

	if err := stream.SendMsg(out); err != nil {
		return fmt.Errorf("send msg: %w", err)
	}
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
