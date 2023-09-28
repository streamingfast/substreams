package server

import (
	"context"

	dgrpcserver "github.com/streamingfast/dgrpc/server"
)

func (s *server) healthzHandler() dgrpcserver.HealthCheck {
	return func(ctx context.Context) (bool, interface{}, error) {
		if s.IsTerminating() {
			return false, nil, nil
		}
		return true, nil, nil
	}
}
