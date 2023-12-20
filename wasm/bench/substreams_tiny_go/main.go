//go:build wasip1

package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"unsafe"

	"github.com/golang/protobuf/proto"
	"github.com/streamingfast/substreams/wasm/bench/substreams_tiny_go/pb"
)

func main() {
	Println("let's do it")
	input, err := readInput()
	//start := time.Now()
	if err != nil {
		panic(fmt.Errorf("reading input: %w", err))
	}

	mapBlockInput := &pb.MapBlockInput{}
	err = proto.Unmarshal(input, mapBlockInput)

	err = mapBlock(mapBlockInput.Block)
	if err != nil {
		panic(fmt.Errorf("mapping block: %w", err))
	}
	//fmt.Println("_duration", time.Since(start))
}

type blockStat struct {
	TrxCount      int
	TransferCount int
	ApprovalCount int
}

func mapBlock(block *pb.Block) error {
	rocketAddress := strings.ToLower("ae78736Cd615f374D3085123A210448E74Fc6393")

	approvalTopic := strings.ToLower("8c5be1e5ebec7d5bd14f71427d1e84f3dd0314c0f7b2291e5b200ac8c7c3b925")
	transferTopic := strings.ToLower("ddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef")

	trxCount := 0
	transferCount := 0
	approvalCount := 0
	for _, trace := range block.TransactionTraces {
		trxCount++
		if trace.Status != 1 {
			continue
		}
		for _, call := range trace.Calls {
			if call.StateReverted {
				continue
			}
			for _, log := range call.Logs {
				l := hex.EncodeToString(log.Address)
				l = strings.ToLower(l)
				if l != rocketAddress || len(log.Topics) == 0 {
					continue
				}
				t := hex.EncodeToString(log.Topics[0])
				t = strings.ToLower(t)
				if t == approvalTopic {
					approvalCount++
				}
				if t == transferTopic {
					transferCount++
				}
			}
		}
	}
	stats := blockStat{
		TrxCount:      trxCount,
		TransferCount: transferCount,
		ApprovalCount: approvalCount,
	}
	data, err := json.Marshal(stats)
	if err != nil {
		return fmt.Errorf("marshalling stats: %w", err)
	}
	_, err = writeOutput(data)
	if err != nil {
		return fmt.Errorf("writing output: %w", err)
	}
	return nil
}

func writeOutput(data []byte) (int, error) {
	return os.Stdout.Write(data)
}

func readInput() ([]byte, error) {
	return io.ReadAll(os.Stdin)
}

//go:wasmimport logger println
//go:noescape
func println(buf unsafe.Pointer, len uint32)

func Println(s string) {
	d := []byte(s)
	println(unsafe.Pointer(&d[0]), uint32(len(d)))
}

//go:wasmimport store get_at
//go:noescape
func get_at(storeIndex uint32, ordinal uint64, keyPtr unsafe.Pointer, keyLen uint32, outputPtr unsafe.Pointer) int32

type ReadStore struct {
	storeIdx uint32
}

func NewReadStore(storeIdx uint32) *ReadStore {
	return &ReadStore{storeIdx: uint32(storeIdx)}
}

func (rs *ReadStore) GetAt(ordinal uint64, key []byte) []byte {
	keyPtr := unsafe.Pointer(&key[0])
	keyLen := uint32(len(key))
	outputPtr := unsafe.Pointer(uintptr(0))
	ret := get_at(rs.storeIdx, ordinal, keyPtr, keyLen, outputPtr)
	if ret != 0 {
		return nil
	}
	return getOutputData(outputPtr)
}

func getOutputData(outputPtr unsafe.Pointer) []byte {
	valuePtr := *(*uint32)(unsafe.Pointer(outputPtr))
	valueLen := *(*uint32)(unsafe.Pointer(uintptr(outputPtr) + uintptr(4)))

	// Convert valuePtr (which is a uint32) to an unsafe.Pointer,
	// and then to a pointer to a byte.
	dataPtr := unsafe.Pointer(uintptr(valuePtr))

	// Construct a slice from dataPtr.
	// We are creating a slice header manually here.
	sliceHeader := &reflect.SliceHeader{
		Data: uintptr(dataPtr),
		Len:  int(valueLen),
		Cap:  int(valueLen),
	}

	// Convert the slice header to a []byte slice.
	return *(*[]byte)(unsafe.Pointer(sliceHeader))
}
