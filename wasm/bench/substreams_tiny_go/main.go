//go:build wasip1

package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	_ "runtime"
	"strings"
	"unsafe"

	"github.com/golang/protobuf/proto"
	pbeth "github.com/streamingfast/substreams/wasm/bench/substreams_tiny_go/pb/sf/ethereum/type/v2"
)

func main() {
	printIt("let's do it")
	//start := time.Now()
	input, err := readInput()
	if err != nil {
		panic(fmt.Errorf("reading input: %w", err))
	}

	//fmt.Println("Input length:", len(input))
	//todo: read args to know what module func to run

	//todo switch on os.Args[1]
	//switch os.Args[1] {
	//case "mapBlock":
	//	param := &MapBlockParam{}
	//	err = proto.Unmarshal(input, param)
	//	if err != nil {
	//		panic(fmt.Errorf("unmarshalling input: %w", err))
	//	}
	//	mapBlock(param.Block, NewReadStore(param.MyStoreIndex))
	//}

	block := &pbeth.Block{}
	err = proto.Unmarshal(input, block)
	if err != nil {
		panic(fmt.Errorf("unmarshalling input: %w", err))
	}
	err = mapBlock(block)
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

func mapBlock(block *pbeth.Block) error {
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
func fuck(buf unsafe.Pointer, len uint32)

func printIt(s string) {
	d := []byte(s)
	fuck(unsafe.Pointer(&d[0]), uint32(len(d)))
}

////go:wasmimport wasi_snapshot_preview1 random_get
////go:noescape
//func random_get(buf unsafe.Pointer, bufLen size) errno
//
//func getRandomData(r []byte) {
//	if random_get(unsafe.Pointer(&r[0]), size(len(r))) != 0 {
//		throw("random_get failed")
//	}
//}
