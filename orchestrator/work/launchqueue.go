package work

import (
	"context"
	"math/rand"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/streamingfast/substreams/orchestrator/stage"
)

// LaunchQueue decides which jobs of one tier1 request may send a request to a tier2, and
// when. A tier2 at its concurrent-request limit refuses the job without doing any work, so
// the job dials again and can land on an instance that has room; when every job of the
// request dials at the same rate, the segment the client reads first is no more likely to
// land than the one it reads last, and a whole request can idle behind one unlucky low
// segment.
//
// The queue holds the jobs a tier2 turned away, plus the ones held back behind them,
// ordered by what the client reads first: lowest segment, then highest stage, the job that
// also produces the partials of the stages under it. Only the first windowSize jobs of
// that queue may dial at all; the ones behind them send nothing and wait. A job leaves the
// queue as soon as a tier2 takes it, which lets exactly one more job start dialing.
//
// A job with nothing queued ahead of it dials immediately and never joins, so a fleet with
// room is paced no differently than without the queue.
type LaunchQueue struct {
	windowSize int

	mu      sync.Mutex
	waiting map[stage.Unit]*launchWaiter
}

type launchWaiter struct {
	refusedAt time.Time // zero until a tier2 turns the job away
	jitter    time.Duration
}

// launchWindowPercent is how much of a request's worker count may be dialing tier2
// instances while the fleet has no room, as a percentage.
var launchWindowPercent = 20

// minLaunchWindow keeps a small request from serializing onto a single job.
const minLaunchWindow = 2

func init() {
	if val := os.Getenv("SUBSTREAMS_WORKER_LAUNCH_WINDOW_PERCENT"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
			launchWindowPercent = parsed
		}
	}
}

func NewLaunchQueue(maxWorkers int) *LaunchQueue {
	windowSize := maxWorkers * launchWindowPercent / 100
	if windowSize < minLaunchWindow {
		windowSize = minLaunchWindow
	}

	return &LaunchQueue{
		windowSize: windowSize,
		waiting:    make(map[stage.Unit]*launchWaiter),
	}
}

// WaitTurn blocks until the job may send its request to a tier2.
func (q *LaunchQueue) WaitTurn(ctx context.Context, unit stage.Unit) error {
	for {
		wait := q.waitFor(unit)
		if wait <= 0 {
			return nil
		}
		// Look again at least once per retry delay: the job's place in the queue moves on
		// its own as the jobs ahead of it get in.
		if wait > workerOverloadedRetryDelay {
			wait = workerOverloadedRetryDelay
		}

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// Refused records that a tier2 turned the job away. The job keeps its place in the queue.
func (q *LaunchQueue) Refused(unit stage.Unit) {
	q.mu.Lock()
	defer q.mu.Unlock()

	waiter, queued := q.waiting[unit]
	if !queued {
		waiter = q.joinLocked(unit)
	}
	waiter.refusedAt = time.Now()
	waiter.jitter = newJitter()
}

// Leave takes the job out of the queue, moving up every job behind it. It is called once
// the job is through to a tier2, and again when the job ends.
func (q *LaunchQueue) Leave(unit stage.Unit) {
	q.mu.Lock()
	defer q.mu.Unlock()

	delete(q.waiting, unit)
}

// waitFor returns how long the job must wait before dialing, putting it in the queue if it
// isn't there yet.
func (q *LaunchQueue) waitFor(unit stage.Unit) time.Duration {
	q.mu.Lock()
	defer q.mu.Unlock()

	position := 0
	for other := range q.waiting {
		if launchesBefore(other, unit) {
			position++
		}
	}

	waiter, queued := q.waiting[unit]
	if !queued {
		if position == 0 {
			// Nothing is waiting for a tier2 ahead of it: the fleet is not pushing back.
			return 0
		}
		waiter = q.joinLocked(unit)
	}

	if position >= q.windowSize {
		// Outside the window: jobs the client needs sooner are still trying to get in, so
		// this one sends nothing at all. Come back when the queue has moved.
		return workerOverloadedRetryDelay
	}
	if waiter.refusedAt.IsZero() {
		return 0
	}
	return time.Until(waiter.refusedAt.Add(workerOverloadedRetryDelay + waiter.jitter))
}

func (q *LaunchQueue) joinLocked(unit stage.Unit) *launchWaiter {
	waiter := &launchWaiter{jitter: newJitter()}
	q.waiting[unit] = waiter
	return waiter
}

// launchesBefore orders jobs by what the client reads first: the lowest segment, and
// within a segment the highest stage, whose job also produces the partials of the stages
// under it. It is the order Stages.NextJob hands jobs out.
func launchesBefore(a, b stage.Unit) bool {
	if a.Segment != b.Segment {
		return a.Segment < b.Segment
	}
	return a.Stage > b.Stage
}

// newJitter keeps jobs refused at the same instant from redialing in lockstep. It is only
// ever added to the delay, never subtracted.
func newJitter() time.Duration {
	if workerOverloadedRetryJitter <= 0 {
		return 0
	}
	return time.Duration(rand.Int63n(int64(workerOverloadedRetryJitter)))
}
