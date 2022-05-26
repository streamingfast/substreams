package state

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/streamingfast/derr"

	"github.com/streamingfast/dstore"
	"go.uber.org/zap"
)

type Info struct {
	LastKVFile        string `json:"last_kv_file"`
	LastKVSavedBlock  uint64 `json:"last_saved_block"`
	RangeIntervalSize uint64 `json:"range_interval_size"`
}

func writeStateInfo(ctx context.Context, store dstore.Store, info *Info) error {
	data, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("marshaling state info: %w", err)
	}

	err = derr.RetryContext(ctx, 3, func(ctx context.Context) error {
		return store.WriteObject(ctx, InfoFileName(), bytes.NewReader(data))
	})
	if err != nil {
		return fmt.Errorf("writing file %s: %w", InfoFileName(), err)
	}

	return nil
}

func readStateInfo(ctx context.Context, store dstore.Store) (*Info, error) {
	var rc io.ReadCloser
	err := derr.RetryContext(ctx, 3, func(ctx context.Context) error {
		var e error
		rc, e = store.OpenObject(ctx, InfoFileName())
		return e
	})
	if err != nil {
		if err == dstore.ErrNotFound {
			return &Info{}, nil
		}
		return nil, fmt.Errorf("opening object %s: %w", InfoFileName(), err)
	}

	defer func(rc io.ReadCloser) {
		err := rc.Close()
		if err != nil {
			zlog.Error("closing object", zap.String("object_name", InfoFileName()), zap.Error(err))
		}
	}(rc)

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("reading data for %s: %w", InfoFileName(), err)
	}

	var info *Info
	err = json.Unmarshal(data, &info)
	if err != nil {
		return nil, fmt.Errorf("unmarshaling state info data: %w", err)
	}

	return info, nil
}

func (b *Builder) Info(ctx context.Context) (*Info, error) {
	if b.info == nil {
		b.infoLock.Lock()
		defer b.infoLock.Unlock()

		if info, err := readStateInfo(ctx, b.Store); err != nil {
			return nil, fmt.Errorf("reading state info for builder %q: %w", b.Name, err)
		} else {
			return info, nil
		}

	}

	return b.info, nil
}
