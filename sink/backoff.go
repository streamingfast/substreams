package sink

import (
	"fmt"

	"github.com/cenkalti/backoff/v4"
)

type BackOffStringer struct{ backoff.BackOff }

func (s BackOffStringer) String() string {
	switch v := s.BackOff.(type) {
	case *backoff.ZeroBackOff:
		return "Retry Immediately"
	case *backoff.StopBackOff:
		return "Stop Immediately"
	case *backoff.ConstantBackOff:
		return fmt.Sprintf("Wait Constantly %s", v.Interval)
	case *backoff.ExponentialBackOff:
		return fmt.Sprintf("Wait Exponentialy (interval: %s, max interval: %s, max elapsed time: %s)", v.InitialInterval, v.MaxInterval, v.MaxElapsedTime)
	default:
		return fmt.Sprintf("%T", v)
	}
}
