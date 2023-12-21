//go:build wasip1

package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/streamingfast/substreams/wasm/bench/substreams_tiny_go/pb"
	"io"
	"log"
	"os"
	"strings"
)

func main() {
	log.Print("let's do it")

	input, err := readInput()
	//start := time.Now()
	if err != nil {
		panic(fmt.Errorf("reading input: %w", err))
	}

	var entrypoint string
	switch len(os.Args) {
	case 1:
		entrypoint = os.Args[0]
	default:
		panic(fmt.Errorf("invalid number of arguments: %d", len(os.Args)))
	}
	fmt.Println("entrypoint", entrypoint)

	switch entrypoint {
	case "mapBlock":
		mapBlockInput := &pb.MapBlockInput{}
		err = proto.Unmarshal(input, mapBlockInput)

		fmt.Println("parameters:", mapBlockInput.Params)
		fmt.Println("read store:", mapBlockInput.ReadStore)
		fmt.Println("read store2:", mapBlockInput.ReadStore2)

		err = mapBlock(mapBlockInput.Block)
		if err != nil {
			panic(fmt.Errorf("mapping block: %w", err))
		}
	}
}

type blockStat struct {
	TrxCount      int
	TransferCount int
	ApprovalCount int
}

func mapBlock(block *pb.Block) error {
	//readStore.GetAt(0, []byte("key_123"))
	readFile("/sys/store/0/read/first/key_123")

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

func readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}
