package state

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"
)

func (b *Builder) WriteState(ctx context.Context) (filename string, err error) {
	zlog.Debug("writing state", zap.String("module", b.Name))

	err = b.writeMergeData()
	if err != nil {
		return "", fmt.Errorf("writing merge values: %w", err)
	}

	kv := stringMap(b.KV) // FOR READABILITY ON DISK

	content, err := json.MarshalIndent(kv, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal kv state: %w", err)
	}

	zlog.Info("write state mode",
		zap.String("store", b.Name),
		zap.Bool("partial", b.partialMode),
		zap.Object("block_range", b.BlockRange),
	)

	if b.partialMode {
		filename, err = b.writePartialState(ctx, content)
	} else {
		filename, err = b.writeState(ctx, content)
	}

	if err != nil {
		return "", fmt.Errorf("writing %s kv for range %s: %w", b.Name, b.BlockRange, err)
	}

	return filename, nil
}

func (b *Builder) writeState(ctx context.Context, content []byte) (string, error) {
	filename := FullStateFileName(b.BlockRange)
	err := b.Store.WriteObject(ctx, filename, bytes.NewReader(content))
	if err != nil {
		return filename, fmt.Errorf("writing state %s for range %s: %w", b.Name, b.BlockRange.String(), err)
	}

	currentInfo, err := b.Info(ctx)
	if err != nil {
		return "", fmt.Errorf("getting builder info: %w", err)
	}

	if currentInfo != nil && currentInfo.LastKVSavedBlock >= b.BlockRange.ExclusiveEndBlock {
		zlog.Debug("skipping info save.")
		return filename, nil
	}

	var info = &Info{
		LastKVFile:        filename,
		LastKVSavedBlock:  b.BlockRange.ExclusiveEndBlock,
		RangeIntervalSize: b.saveInterval,
	}
	err = writeStateInfo(ctx, b.Store, info)
	if err != nil {
		return "", fmt.Errorf("writing state info for builder %q: %w", b.Name, err)
	}

	b.info = info
	zlog.Debug("state file written", zap.String("module_name", b.Name), zap.Object("block_range", b.BlockRange), zap.String("file_name", filename))

	return filename, err
}

func (b *Builder) writePartialState(ctx context.Context, content []byte) (string, error) {
	filename := PartialFileName(b.BlockRange)
	return filename, b.Store.WriteObject(ctx, filename, bytes.NewReader(content))
}
