package state

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/streamingfast/bstream"
	"github.com/streamingfast/dhammer"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/shutter"
	sink "github.com/streamingfast/substreams/sink"
	"github.com/streamingfast/substreams/sink/sql/db_changes/bundler/writer"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

var _ Store = (*FileStateStore)(nil)

type FileStateStore struct {
	*shutter.Shutter

	startOnce sync.Once

	outputPath  string
	outputStore dstore.Store
	uploadQueue *dhammer.Nailer

	logger *zap.Logger

	state *FileState
}

func NewFileStateStore(
	outputPath string,
	outputStore dstore.Store,
	logger *zap.Logger,
) (*FileStateStore, error) {
	s := &FileState{}

	content, err := os.ReadFile(outputPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read file: %w", err)
	}
	if err != nil && os.IsNotExist(err) {
		s = newFileState()
	}

	if err := yaml.Unmarshal(content, s); err != nil {
		return nil, fmt.Errorf("unmarshal state file %q: %w", outputPath, err)
	}
	outputStore.SetOverwrite(true)
	f := &FileStateStore{
		Shutter:     shutter.New(),
		outputPath:  outputPath,
		outputStore: outputStore,
		state:       s,
		logger:      logger,
	}
	f.uploadQueue = dhammer.NewNailer(5, f.uploadCursor, dhammer.NailerLogger(logger))
	return f, nil
}

func (s *FileStateStore) Start(ctx context.Context) {
	s.OnTerminating(func(err error) {
		s.logger.Info("shutting down file cursor", zap.String("store", s.outputPath), zap.Error(err))
		s.Close()
	})

	s.uploadQueue.Start(ctx)

	go func() {
		for v := range s.uploadQueue.Out {
			bf := v.(*cursorFile)
			s.logger.Debug("uploaded file", zap.String("filename", bf.name))
		}
		if s.uploadQueue.Err() != nil {
			s.Shutdown(fmt.Errorf("upload queue failed: %w", s.uploadQueue.Err()))
		}
	}()

	s.uploadQueue.OnTerminating(func(err error) {
		s.Shutdown(fmt.Errorf("upload queue failed: %w", s.uploadQueue.Err()))
	})
}

type cursorFile struct {
	name string
	file writer.Uploadeable
}

func (s *FileStateStore) uploadCursor(ctx context.Context, v interface{}) (interface{}, error) {
	bf := v.(*cursorFile)

	outputPath, err := bf.file.Upload(ctx, s.outputStore)
	if err != nil {
		return nil, fmt.Errorf("unable to upload: %w", err)
	}
	s.logger.Debug("boundary file uploaded",
		zap.String("boundary", bf.name),
		zap.String("output_path", outputPath),
	)

	return bf, nil
}

type localFile struct {
	localFilePath  string
	outputFilename string
}

func (l *localFile) Upload(ctx context.Context, store dstore.Store) (string, error) {
	if err := store.PushLocalFile(ctx, l.localFilePath, l.outputFilename); err != nil {
		return "", fmt.Errorf("pushing  object: %w", err)
	}
	return store.ObjectPath(l.outputFilename), nil
}

func (s *FileStateStore) UploadCursor(saveable Saveable) {
	s.uploadQueue.In <- &cursorFile{
		name: "cursor.yaml",
		file: saveable.GetUploadeable(),
	}
}

func (s *FileStateStore) Close() {
	s.uploadQueue.Close()
	s.logger.Debug("waiting till queue is drained")
	s.uploadQueue.WaitUntilEmpty(context.Background())
}

func (s *FileStateStore) ReadCursor(ctx context.Context) (cursor *sink.Cursor, err error) {
	fl, err := s.outputStore.OpenObject(ctx, "state.yaml")
	if err != nil && err != dstore.ErrNotFound {
		return nil, fmt.Errorf("opening csv: %w", err)
	}

	if err != nil && err == dstore.ErrNotFound {
		s.state = newFileState()
	} else {
		defer fl.Close()
		buf := new(bytes.Buffer)
		buf.ReadFrom(fl)
		content := buf.Bytes()

		if err := yaml.Unmarshal(content, s.state); err != nil {
			return nil, fmt.Errorf("unmarshal state file %q: %w", s.outputPath, err)
		}
	}

	return sink.NewCursor(s.state.Cursor)
}

func (s *FileStateStore) NewBoundary(boundary *bstream.Range) {
	s.state.ActiveBoundary.StartBlockNumber = boundary.StartBlock()
	s.state.ActiveBoundary.EndBlockNumber = *boundary.EndBlock()
}

func (s *FileStateStore) SetCursor(cursor *sink.Cursor) {
	s.startOnce.Do(func() {
		restartAt := time.Now()
		if s.state.StartedAt.IsZero() {
			s.state.StartedAt = restartAt
		}
		s.state.RestartedAt = restartAt
	})

	s.state.Cursor = cursor.String()
	s.state.Block = BlockState{
		ID:     cursor.Block().ID(),
		Number: cursor.Block().Num(),
	}
}

func (s *FileStateStore) GetState() (Saveable, error) {
	cnt, err := yaml.Marshal(s.state)
	if err != nil {
		return nil, fmt.Errorf("marshall: %w", err)
	}
	return &stateInstance{
		data: cnt,
		path: s.outputPath + "-" + s.state.Block.ID,
	}, nil
}

type FileState struct {
	Cursor         string         `yaml:"cursor" json:"cursor"`
	Block          BlockState     `yaml:"block" json:"block"`
	ActiveBoundary ActiveBoundary `yaml:"active_boundary" json:"active_boundary"`

	StartedAt   time.Time `yaml:"started_at,omitempty" json:"started_at,omitempty"`
	RestartedAt time.Time `yaml:"restarted_at,omitempty" json:"restarted_at,omitempty"`
}

func newFileState() *FileState {
	return &FileState{
		Cursor: "",
		Block:  BlockState{"", 0},
	}
}

type BlockState struct {
	ID     string `yaml:"id" json:"id"`
	Number uint64 `yaml:"number" json:"number"`
}

type ActiveBoundary struct {
	StartBlockNumber uint64 `yaml:"start_block_number"  json:"start_block_number"`
	EndBlockNumber   uint64 `yaml:"end_block_number"  json:"end_block_number"`
}

type stateInstance struct {
	data []byte
	path string
}

func (s *stateInstance) GetUploadeable() writer.Uploadeable {
	return &localFile{
		localFilePath:  s.path,
		outputFilename: "state.yaml",
	}
}

func (s *stateInstance) Save() error {
	if err := os.WriteFile(s.path, s.data, os.ModePerm); err != nil {
		return fmt.Errorf("unable to write state file: %w", err)
	}
	return nil
}
