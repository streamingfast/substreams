package subgraph

import (
	"fmt"
	"time"

	graphnode "github.com/streamingfast/substreams/graph-node"
)

// Intrinsics is per subgraph and should be unique for each subgraph. The underlying implementation
// should know about its surrounding context to know when to close when at which block it's currently
// at.
//
// It's expected that the implementation will be called by one go routine at a time.
type Intrinsics interface {
	/// Entities

	Save(entity graphnode.Entity) error
	Load(entity graphnode.Entity) error
	LoadAllDistinct(model graphnode.Entity, blockNum uint64) ([]graphnode.Entity, error)
	Remove(entity graphnode.Entity) error

	/// Block

	// Block returns the current block being processed by your subgraph handler.
	Block() BlockRef

	/// Reproc
	Step() int
	StepBelow(step int) bool
	StepAbove(step int) bool

	/// JSON-RPC
	RPC(calls []*RPCCall) ([]*RPCResponse, error)
}

type BlockRef interface {
	ID() string
	Number() uint64
	Timestamp() time.Time
}

type RPCCall struct {
	ToAddr          string
	MethodSignature string // ex: "name() (string)"
}

func (c *RPCCall) ToString() string {
	return fmt.Sprintf("%s:%s", c.ToAddr, c.MethodSignature)
}

type RPCResponse struct {
	Decoded       []interface{}
	Raw           string
	DecodingError error
	CallError     error // always deterministic
}
