package lib

import (
	"encoding/hex"
	"fmt"
	"log"
	"strings"

	"github.com/streamingfast/substreams/wasm/bench/substreams_wasi_go/pb"
)

func mapBlock(inputs *MapBlockInput) (*pb.MapBlockOutput, error) {
	rocketAddress := strings.ToLower("ae78736Cd615f374D3085123A210448E74Fc6393")

	approvalTopic := strings.ToLower("8c5be1e5ebec7d5bd14f71427d1e84f3dd0314c0f7b2291e5b200ac8c7c3b925")
	transferTopic := strings.ToLower("ddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef")

	v, err := inputs.readStore1.GetFirst("key_123")
	if err != nil {
		return nil, err
	}
	if v != "value_123" {
		return nil, fmt.Errorf("expected value_123, got %q", v)
	}
	log.Print("got value_123")

	trxCount := 0
	transferCount := 0
	approvalCount := 0
	for _, trace := range inputs.block.TransactionTraces {
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

	output := &pb.MapBlockOutput{
		TrxCount:      uint32(trxCount),
		TransferCount: uint32(transferCount),
		ApprovalCount: uint32(approvalCount),
	}

	return output, nil
}
