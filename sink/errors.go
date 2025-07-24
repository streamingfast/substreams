package sink

import (
	"errors"
)

var ErrBackOffExpired = errors.New("unable to complete work within backoff time limit")
