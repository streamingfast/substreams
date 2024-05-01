package execout

import (
	"errors"

	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

type ExecutionOutputGetter interface {
	Len() int
	Clock() *pbsubstreams.Clock
	Get(name string) (value []byte, cached bool, err error)
	IsSkippedFromIndex(moduleName string) bool
}

type ExecutionOutputSetter interface {
	Set(name string, value []byte, isSkippedFromIndex bool) (err error)
	SetFileOutput(name string, value []byte, isSkippedFromIndex bool) (err error)
}

// ExecutionOutput gets/sets execution output for a given graph at a given block
type ExecutionOutput interface {
	ExecutionOutputGetter
	ExecutionOutputSetter
}

var ErrNotFound = errors.New("inputs module value not found")
