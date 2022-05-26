package orchestrator

import (
	"sync"
	"time"
)

type Sleeper struct {
	baseIntervalMilliseconds int64

	currentintervalMilliseconds int64
	sleepCount                  int

	mutex sync.RWMutex
}

func NewSleeper(baseIntervalMilliseconds int64) *Sleeper {
	return &Sleeper{
		baseIntervalMilliseconds:    baseIntervalMilliseconds,
		currentintervalMilliseconds: baseIntervalMilliseconds,
	}
}

func (s *Sleeper) Sleep() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	time.Sleep(time.Duration(s.currentintervalMilliseconds) * time.Millisecond)

	s.sleepCount++
	s.currentintervalMilliseconds += int64(s.sleepCount) * s.baseIntervalMilliseconds
}

func (s *Sleeper) Reset() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.sleepCount = 0
	s.currentintervalMilliseconds = s.baseIntervalMilliseconds
}
