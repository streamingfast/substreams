//go:build wasip1

package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/streamingfast/substreams/wasm/bench/substreams_wasi_go/pb"
)

func main() {
	start := time.Now()
	log.Print("let's do it")
	log.Print("start: ", start)

	input, err := readInput()
	if err != nil {
		panic(fmt.Errorf("reading input: %w", err))
	}
	log.Print("input length: ", len(input))
	log.Print("read input duration: ", time.Since(start))

	//data, err := readFile("/sys/stores/0/read/first/key_123")
	//if err != nil {
	//	panic(fmt.Errorf("reading store: %w", err))
	//	//return fmt.Errorf("reading store: %w", err)
	//}
	//fmt.Println("read store:", string(data))

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

		protoStart := time.Now()
		log.Print("proto start: ", protoStart)
		mapBlockInput := &pb.MapBlockInput{}
		err = mapBlockInput.UnmarshalVT(input)
		//err = proto.Unmarshal(input, mapBlockInput)
		if err != nil {
			panic(fmt.Errorf("unmarshalling args: %w", err))
		}
		log.Print("proto duration: ", time.Since(protoStart))

		log.Print("parameters: ", mapBlockInput.Params)
		log.Print("read store: ", mapBlockInput.ReadStore)
		log.Print("read store2: ", mapBlockInput.ReadStore2)
		log.Print("block: ", mapBlockInput.Block.Number)
		err = mapBlock(mapBlockInput.Block)
		if err != nil {
			panic(fmt.Errorf("mapping block: %w", err))
		}
	}

	log.Print("total duration: ", time.Since(start))
}

func readAll(r io.Reader) ([]byte, error) {
	b := make([]byte, 0, 1024*1024)
	count := 0
	for {
		count++
		n, err := r.Read(b[len(b):cap(b)])
		log.Print("Read count: ", count)
		b = b[:len(b)+n]
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			return b, err
		}

		if len(b) == cap(b) {
			// Add more capacity (let append pick how much).
			b = append(b, 0)[:len(b)]
		}
	}
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
	return readAll(os.Stdin)
}

func readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}
