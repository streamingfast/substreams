package pipeline

import (
	"context"
	"errors"
	"fmt"
	"github.com/streamingfast/bstream/stream"
	errors2 "github.com/streamingfast/substreams/errors"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"io"
	"strings"
)

func (p *Pipeline) StreamEndedWithErr(streamSrv pbsubstreams.Stream_BlocksServer, err error) errors2.GRPCError {
	if errors.Is(err, stream.ErrStopBlockReached) {
		p.reqCtx.Logger().Debug("stream of blocks reached end block, triggering StoreSave",
			zap.Uint64("stop_block_num", p.reqCtx.StopBlockNum()),
		)

		// treat StopBlockNum as possible boundaries (if chain has holes...)
		if err := p.FlushStores(p.reqCtx.StopBlockNum()); err != nil {
			return errors2.NewBasicErr(status.Errorf(codes.Internal, "handling store save boundaries: %s", err), err)
		}
	}

	if errors.Is(err, io.EOF) || errors.Is(err, stream.ErrStopBlockReached) {
		var d []string
		for _, rng := range p.partialsWritten {
			d = append(d, fmt.Sprintf("%d-%d", rng.StartBlock, rng.ExclusiveEndBlock))
		}
		partialsWritten := []string{strings.Join(d, ",")}
		p.reqCtx.Logger().Info("setting trailer", zap.Strings("ranges", partialsWritten))
		streamSrv.SetTrailer(metadata.MD{"substreams-partials-written": partialsWritten})
		return nil
	}

	if errors.Is(err, context.Canceled) {
		return errors2.NewErrContextCanceled(err)
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return errors2.NewErrDeadlineExceeded(err)
	}

	var errInvalidArg *stream.ErrInvalidArg
	if errors.As(err, &errInvalidArg) {
		return errors2.NewBasicErr(status.Error(codes.InvalidArgument, errInvalidArg.Error()), err)
	}

	var errSendBlock *errors2.ErrSendBlock
	if errors.As(err, &errSendBlock) {
		p.reqCtx.Logger().Info("unable to send block probably due to client disconnecting", zap.Error(errSendBlock.Inner))
		return *errSendBlock
	}

	p.reqCtx.Logger().Info("unexpected stream of blocks termination", zap.Error(err))
	return errors2.NewBasicErr(status.Errorf(codes.Internal, "unexpected termination: %s", err), err)
}
