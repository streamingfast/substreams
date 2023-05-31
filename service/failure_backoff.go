package service

import "time"

// fail fast when the exact same request has already failed twice, preventing waste of tier2 resources
var FailureBlacklistMinimalCount = 0
var FailureBlacklistDuration = time.Minute * 10

// hold the incoming request for this duration before answering with an error if the client keeps retrying
var FailureForcedBackoffIncrement = time.Millisecond * 500
var FailureForcedBackoffLimit = time.Second * 30

type recordedFailure struct {
	lastAt        time.Time
	count         int
	forcedBackoff time.Duration
	lastError     error
}

func (s *Tier1Service) errorFromRecordedFailure(id string) error {
	s.failedRequestsLock.RLock()
	defer s.failedRequestsLock.RUnlock()
	if failure, ok := s.failedRequests[id]; ok {
		if failure.count > FailureBlacklistMinimalCount {
			if time.Since(failure.lastAt) < FailureBlacklistDuration {
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

func (s *Tier1Service) recordFailure(requestID string, err error) {
	s.failedRequestsLock.Lock()
	defer s.failedRequestsLock.Unlock()
	failure := s.failedRequests[requestID]
	if failure == nil {
		failure = &recordedFailure{}
		s.failedRequests[requestID] = failure
	}
	failure.lastAt = time.Now()
	failure.lastError = err
	failure.count++
}
