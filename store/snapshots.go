package store

import (
	"context"
	"fmt"
	"github.com/streamingfast/derr"
)

func (s *BaseStore) ListSnapshotFiles(ctx context.Context) (files []*FileInfo, err error) {
	err = derr.RetryContext(ctx, 3, func(ctx context.Context) error {
		if err := s.store.Walk(ctx, "", func(filename string) (err error) {
			fileInfo, ok := parseFileName(filename)
			if !ok {
				return nil
			}
			files = append(files, fileInfo)
			return nil
		}); err != nil {
			return fmt.Errorf("walking snapshots: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}
