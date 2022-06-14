package orchestrator

import (
	"fmt"
	"strings"

	"go.uber.org/zap/zapcore"
)

type chunk struct {
	start       uint64
	end         uint64 // exclusive end
	tempPartial bool   // for off-of-bound stores (like ending in 1123, and not on 1000)
}

func (c *chunk) String() string {
	var add string
	if c.tempPartial {
		add = "TMP:"
	}
	return fmt.Sprintf("%s%d-%d", add, c.start, c.end)
}
func (c *chunk) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddUint64("start_block", c.start)
	enc.AddUint64("end_block", c.end)

	return nil
}

type chunks []*chunk

func (c chunks) String() string {
	var sc []string
	for _, s := range c {
		var add string
		if s.tempPartial {
			add = "TMP:"
		}
		sc = append(sc, fmt.Sprintf("%s%d-%d", add, s.start, s.end))
	}
	return strings.Join(sc, ", ")
}
