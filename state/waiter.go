package state

import (
	"context"
	"fmt"
	"github.com/streamingfast/dstore"
	"regexp"
	"strconv"
	"time"

	"go.uber.org/zap"

	"github.com/abourget/llerrgroup"
)

const WaiterSleepInterval = 5 * time.Second

type FileWaiter struct {
	builders          []*Builder
	targetBlockNumber uint64
}

func NewFileWaiter(targetStartBlock uint64, builders []*Builder) *FileWaiter {
	w := &FileWaiter{
		builders:          builders,
		targetBlockNumber: targetStartBlock,
	}

	return w
}

func (w *FileWaiter) Wait(ctx context.Context, requestStartBlock uint64, moduleStartBlock uint64) error {
	eg := llerrgroup.New(len(w.builders))
	for _, builder := range w.builders {
		if eg.Stop() {
			continue // short-circuit the loop if we got an error
		}
		b := builder
		eg.Go(func() error {
			return <-w.wait(ctx, requestStartBlock, moduleStartBlock, b.Name, b.Store)
		})
	}

	if err := eg.Wait(); err != nil {
		return err
	}

	return nil
}

func WaitPartial(ctx context.Context, storeName string, store dstore.Store, startBlock, endBlock uint64) <-chan error {
	done := make(chan error)

	go func() {
		defer close(done)

		for {
			//check context
			select {
			case <-ctx.Done():
				done <- &fileWaitResult{ctx.Err()}
				return
			default:
				//
			}

			fileName := PartialFileName(storeName, startBlock, endBlock)
			zlog.Info("looking for partial file:", zap.String("file_name", fileName))
			exists, err := store.FileExists(ctx, fileName)
			if err != nil {
				done <- &fileWaitResult{fmt.Errorf("checking if file %s exists, : %w", fileName, err)}
				return
			}

			if exists {
				return
			}

			time.Sleep(WaiterSleepInterval)
		}
	}()

	return done
}

func WaitKV(ctx context.Context, storeName string, store dstore.Store, moduleStartBlock, endBlock uint64) <-chan error {
	done := make(chan error)

	go func() {
		defer close(done)

		for {
			//check context
			select {
			case <-ctx.Done():
				done <- &fileWaitResult{ctx.Err()}
				return
			default:
				//
			}

			fileName := StateFileName(storeName, endBlock, moduleStartBlock)
			zlog.Info("looking for kv file:", zap.String("file_name", fileName))
			exists, err := store.FileExists(ctx, fileName)
			if err != nil {
				done <- &fileWaitResult{fmt.Errorf("checking if file %s exists, : %w", fileName, err)}
				return
			}

			if exists {
				return
			}

			time.Sleep(WaiterSleepInterval)
		}
	}()

	return done
}

func (w *FileWaiter) wait(ctx context.Context, requestStartBlock uint64, moduleStartBlock uint64, storeName string, store dstore.Store) <-chan error {
	done := make(chan error)
	waitForBlockNum := requestStartBlock
	go func() {
		defer close(done)

		for {
			//check context
			select {
			case <-ctx.Done():
				done <- &fileWaitResult{ctx.Err()}
				return
			default:
				//
			}

			if w.targetBlockNumber <= moduleStartBlock {
				return
			}

			prefix := StateFilePrefix(storeName, waitForBlockNum)
			files, err := store.ListFiles(ctx, prefix, "", 1)
			if err != nil {
				done <- &fileWaitResult{fmt.Errorf("listing file with prefix %s, : %w", prefix, err)}
				return
			}

			found := len(files) == 1
			if !found {
				time.Sleep(WaiterSleepInterval)
				fmt.Printf("waiting for store %s to complete processing to block %d\n", store, w.targetBlockNumber)
				continue
			}

			fileinfo, ok := ParseFileName(files[0])
			if !ok {
				done <- &fileWaitResult{fmt.Errorf("could not parse filename %s", files[0])}
				return
			}

			if fileinfo.Partial {
				waitForBlockNum = fileinfo.StartBlock
				if fileinfo.StartBlock == moduleStartBlock {
					return
				}
				continue
			}

			return //we are done because we found a kv file.
		}
	}()

	return done
}

func pathToState(ctx context.Context, storeName string, store dstore.Store, requestStartBlock uint64, moduleStartBlock uint64) ([]string, error) {
	var out []string
	nextBlockNum := requestStartBlock
	for {
		//check context
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			//
		}

		prefix := StateFilePrefix(storeName, nextBlockNum)
		files, err := store.ListFiles(ctx, prefix, "", 2)
		if err != nil {
			return nil, fmt.Errorf("listing file with prefix %s, : %w", prefix, err)
		}

		found := len(files) >= 1
		if !found {
			return nil, fmt.Errorf("file not found to prefix %s for module %s", prefix, storeName)
		}

		foundFile := files[0]
		switch len(files) {
		case 2:
			leftFileInfo, _ := ParseFileName(files[0])
			partialLeft := leftFileInfo.Partial

			rightFileInfo, _ := ParseFileName(files[1])
			partialRight := rightFileInfo.Partial

			if partialLeft && partialRight {
				return nil, fmt.Errorf("found two partial files %s and %s", files[0], files[1])
			}
			if !partialRight {
				foundFile = files[1]
			}
		}

		out = append(out, foundFile)

		fileinfo, ok := ParseFileName(foundFile)
		start, partial := fileinfo.StartBlock, fileinfo.Partial
		if !ok {
			return nil, fmt.Errorf("could not parse filename %s", files[0])
		}

		//todo: validate that start block match configured kv block range.
		if partial {
			nextBlockNum = start
			//todo 2.1: if start block from partial is the module start block we are done waiting
			if start == moduleStartBlock {
				reversePathToState(out)
				return out, nil
			}
			continue
		}

		break //we are done because we found a kv file.
	}

	reversePathToState(out)
	return out, nil
}
func reversePathToState(input []string) {
	for i, j := 0, len(input)-1; i < j; i, j = i+1, j-1 {
		input[i], input[j] = input[j], input[i]
	}
}

type fileWaitResult struct {
	Err error
}

func (f fileWaitResult) Error() string {
	return f.Err.Error()
}

var fullKVRegex *regexp.Regexp
var partialKVRegex *regexp.Regexp
var stateFileRegex *regexp.Regexp

func init() {
	stateFileRegex = regexp.MustCompile(`([\w]+)-([\d]+)-([\d]+)\.(kv|partial)`)
}

type FileInfo struct {
	ModuleName string
	StartBlock uint64
	EndBlock   uint64
	Partial    bool
}

func ParseFileName(filename string) (*FileInfo, bool) {
	res := stateFileRegex.FindAllStringSubmatch(filename, 1)
	if len(res) != 1 {
		return nil, false
	}

	module := res[0][1]
	end := uint64(mustAtoi(res[0][2]))
	start := uint64(mustAtoi(res[0][3]))
	partial := res[0][4] == "partial"

	return &FileInfo{
		ModuleName: module,
		StartBlock: start,
		EndBlock:   end,
		Partial:    partial,
	}, true
}

func mustAtoi(s string) int {
	i, err := strconv.Atoi(s)
	if err != nil {
		panic(err)
	}
	return i
}
