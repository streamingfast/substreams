package main

import (
	"fmt"
	"strings"
	"time"

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
	service, err := NewService("./cmd/substreams/dyntest/eth_xfer.wasm")
	if err != nil {
		return fmt.Errorf("init service: %w", err)
	}

	// TODO: generate this ServiceDesc struct from the
	// loaded .proto files we'd receive (from within the Substreams `spkg` ?)
	// or reuse the dynamic protobuf loading we use in `substreams`.
	var v interface{}
	srv.RegisterService(&grpc.ServiceDesc{
		ServiceName: "sf.mycustomer.v1.Eth",
		HandlerType: service,
		Methods:     []grpc.MethodDesc{},
		Streams: []grpc.StreamDesc{
			{
				StreamName:    "Transfers",
				Handler:       service.New("sf.mycustomer.v1.Eth.Transfers").handle,
				ServerStreams: true,
			},
		},
		Metadata: "sf/mycustomer/v1/eth.proto",
	}, v)

	fmt.Println("Listening on :7878")
	s.Launch(":7878")

	return nil
}

type Service struct {
	bg *bindgen.Bindgen
	vm *wasmedge.VM
}

func NewService(wasmFile string) (*Service, error) {
	// See: https://github.com/second-state/WasmEdge-go-examples/blob/master/wasmedge-bindgen/go_BindgenFuncs/bindgen_funcs.go
	wasmedge.SetLogErrorLevel()
	conf := wasmedge.NewConfigure(wasmedge.WASI)
	vm := wasmedge.NewVMWithConfig(conf)
	wasi := vm.GetImportModule(wasmedge.WASI)
	wasi.InitWasi(nil, nil, nil)
	if err := vm.LoadWasmFile(wasmFile); err != nil {
		return nil, fmt.Errorf("load wasm: %w", err)
	}
	if err := vm.Validate(); err != nil {
		return nil, fmt.Errorf("validate: %w", err)
	}

	bg := bindgen.New(vm)
	if err := bg.GetVm().Instantiate(); err != nil {
		return nil, fmt.Errorf("error instantiating VM: %w", err)
	}
	return &Service{
		bg: bg,
		vm: vm,
	}, nil
}

type Handler struct {
	exportName string
	service    *Service
}

func (s *Service) New(streamName string) *Handler {
	exportName := strings.Replace(streamName, ".", "_", -1)
	exportName = strings.Replace(exportName, "/", "_", -1)
	exportName = strings.ToLower(exportName)
	// TODO: do validation that there are only letters and digits left

	return &Handler{
		exportName: exportName,
		service:    s,
	}
}
func (h *Handler) handle(server interface{}, stream grpc.ServerStream) error {
	t0 := time.Now()
	defer func() {
		fmt.Println("Timing:", time.Since(t0))
	}()

	m := NewPassthroughBytes()
	if err := stream.RecvMsg(m); err != nil {
		return err
	}

	res, _, err := h.service.bg.Execute(h.exportName, m.Bytes)
	if err != nil {
		return fmt.Errorf("executing func %q: %w", h.exportName, err)
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
