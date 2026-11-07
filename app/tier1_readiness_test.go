package app

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type fakeHead struct {
	headTime time.Time
	err      error
}

func (f fakeHead) HeadInfo() (uint64, string, time.Time, uint64, error) {
	return 1, "1a", f.headTime, 0, f.err
}

func TestTier1App_LiveHeadTooOld(t *testing.T) {
	tests := []struct {
		name       string
		maxLatency time.Duration
		head       *fakeHead
		expected   bool
	}{
		{"check off", 0, &fakeHead{headTime: time.Now().Add(-time.Hour)}, false},
		{"no live source", time.Minute, nil, false},
		{"recent head", time.Minute, &fakeHead{headTime: time.Now().Add(-10 * time.Second)}, false},
		{"old head", time.Minute, &fakeHead{headTime: time.Now().Add(-2 * time.Minute)}, true},
		{"head unknown", time.Minute, &fakeHead{err: errors.New("not ready")}, true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			app := &Tier1App{config: &Tier1Config{ReadinessMaxLatency: test.maxLatency}}
			if test.head != nil {
				var head headInfoer = *test.head
				app.liveHead.Store(&head)
			}

			assert.Equal(t, test.expected, app.liveHeadTooOld())
		})
	}
}
