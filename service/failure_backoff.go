package service

import (
	"regexp"
	"strconv"
	"time"

	"github.com/streamingfast/bstream"
)

// fail fast when the exact same request has already failed twice, preventing waste of tier2 resources
var FailureBlacklistMinimalCount = 0
var FailureBlacklistDuration = time.Minute * 10

// hold the incoming request for this duration before answering with an error if the client keeps retrying
var FailureForcedBackoffIncrement = time.Millisecond * 500
var FailureForcedBackoffLimit = time.Second * 30

type recordedFailure struct {
	lastAt        time.Time
	atBlock       uint64
	count         int
	forcedBackoff time.Duration
	lastError     error
}

func (s *Tier1Service) errorFromRecordedFailure(id string, isProductionMode bool, startBlock int64, startCursor string) error {
	if startBlock < 0 {
		return nil
	}
	s.failedRequestsLock.RLock()
	defer s.failedRequestsLock.RUnlock()
	if failure, ok := s.failedRequests[id]; ok {
		if failure.count > FailureBlacklistMinimalCount {
			if time.Since(failure.lastAt) < FailureBlacklistDuration {

				// dev-mode requests below the failure point will still be processed on tier1
				if !isProductionMode {
					if uint64(startBlock) < failure.atBlock {
						cur, err := bstream.CursorFromOpaque(startCursor)
						if err != nil || cur.Block.Num() < failure.atBlock {
							return nil
						}
					}
				}

				time.Sleep(failure.forcedBackoff)
				if failure.forcedBackoff < FailureForcedBackoffLimit {
					failure.forcedBackoff += FailureForcedBackoffIncrement
				}
				return failure.lastError
			} else {
				delete(s.failedRequests, id)
			}
		}
	}
	return nil
}

// Error: rpc error: code = InvalidArgument desc = step new irr: handler step new: execute modules: applying executor results ... store wasm call: block 300: module "store_eth_stats": wasm execution failed ...
var blockFailureRE = regexp.MustCompile(`store wasm call: block ([0-9]*): module "([^"]*)"`)

func (s *Tier1Service) recordFailure(requestID string, err error) {
	s.failedRequestsLock.Lock()
	defer s.failedRequestsLock.Unlock()
	failure := s.failedRequests[requestID]
	if failure == nil {
		failure = &recordedFailure{}
		s.failedRequests[requestID] = failure
	}
	if out := blockFailureRE.FindAllStringSubmatch(err.Error(), -1); out != nil {
		if len(out[0]) == 3 {
			if val, err := strconv.ParseUint(out[0][1], 10, 64); err == nil {
				failure.atBlock = val
			}
		}
	}

	failure.lastAt = time.Now()
	failure.lastError = err
	failure.count++
}
