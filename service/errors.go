package service

import (
	"fmt"
)

type ErrSendBlock struct {
	inner error
}

func NewErrSendBlock(inner error) ErrSendBlock {
	return ErrSendBlock{
		inner: inner,
	}
}

func (e ErrSendBlock) Error() string {
	return fmt.Sprintf("grpc send error: %s", e.inner)
}
