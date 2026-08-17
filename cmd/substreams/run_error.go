package main

import (
	"fmt"
	"strings"

	"github.com/streamingfast/substreams/internal/formatx"
)

// sessionWorkReporter is what explainRunError needs out of the UI: the work the server said it
// had to do. Narrowed to an interface so the explanation can be tested on its own.
type sessionWorkReporter interface {
	SessionWork() (total, prepareStores uint64, ok bool)
}

// limitProcessedBlocksFlag is the guard the server enforces and the client sets. It defaults
// to 10,000 blocks, which any store-backed package exceeds immediately, so tripping it is the
// most common way a first run fails.
const limitProcessedBlocksFlag = "limit-processed-blocks"

// explainRunError replaces the raw gRPC status of failures we can say something useful about.
//
// The server already explains the processed-blocks limit, but it does so in an unformatted
// sentence that ends without telling the user which knob to turn — and the numbers it quotes
// are ones the client was itself told during session init.
func explainRunError(err error, work sessionWorkReporter, limit uint64) error {
	if err == nil || !strings.Contains(err.Error(), limitProcessedBlocksFlag) {
		return err
	}

	// One line, no hard wrapping: the terminal knows its own width, we do not.
	total, prepareStores, ok := work.SessionWork()
	if !ok {
		return fmt.Errorf("this request processes more blocks than --%s allows (%s): raise it, or remove the guard entirely with --%s 0",
			limitProcessedBlocksFlag, formatx.Integer(limit), limitProcessedBlocksFlag)
	}

	needed := formatx.Integer(total) + " blocks"
	if prepareStores != 0 {
		needed += fmt.Sprintf(" (%s of them to prepare the stores)", formatx.Integer(prepareStores))
	}

	return fmt.Errorf("--%s is set to %s, but this request needs to process %s: raise it with --%s %d, or remove the guard entirely with --%s 0",
		limitProcessedBlocksFlag, formatx.Integer(limit), needed,
		limitProcessedBlocksFlag, suggestedLimit(total), limitProcessedBlocksFlag)
}

// suggestedLimit rounds the required work up to something a person would type, so the value
// offered stays valid if the chain moves a little before the command is re-run.
func suggestedLimit(total uint64) uint64 {
	step := uint64(1_000)
	for _, candidate := range []uint64{10_000, 100_000, 1_000_000} {
		if total >= candidate*10 {
			step = candidate
		}
	}

	return ((total / step) + 1) * step
}
