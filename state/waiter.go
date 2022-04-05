package state

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/abourget/llerrgroup"
)

const WaiterSleepInterval = 5 * time.Second

type FileWaiter struct {
	stores            []*Store
	targetBlockNumber uint64
}

func NewFileWaiter(targetStartBlock uint64, stores []*Store) *FileWaiter {
	w := &FileWaiter{
		stores:            stores,
		targetBlockNumber: targetStartBlock,
	}

	return w
}

func (w *FileWaiter) Wait(ctx context.Context, requestStartBlock uint64, moduleStartBlock uint64) error {
	eg := llerrgroup.New(len(w.stores))
	for _, store := range w.stores {
		if eg.Stop() {
			continue // short-circuit the loop if we got an error
		}

		eg.Go(func() error {
			return <-w.wait(ctx, requestStartBlock, moduleStartBlock, store)
		})
	}

	if err := eg.Wait(); err != nil {
		return err
	}

	return nil
}

func WaitKV(ctx context.Context, store *Store, endBlock uint64) <-chan error {
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

			fileName := store.StateFileName(endBlock)
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

func (w *FileWaiter) wait(ctx context.Context, requestStartBlock uint64, moduleStartBlock uint64, store *Store) <-chan error {
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

			prefix := store.StateFilePrefix(waitForBlockNum)
			files, err := store.ListFiles(ctx, store.StateFilePrefix(waitForBlockNum), "", 1)
			if err != nil {
				done <- &fileWaitResult{fmt.Errorf("listing file with prefix %s, : %w", prefix, err)}
				return
			}

			found := len(files) == 1
			if !found {
				time.Sleep(WaiterSleepInterval)
				fmt.Printf("waiting for store %s to complete processing to block %d\n", store.Name, w.targetBlockNumber)
				continue
			}

			ok, start, _, partial := parseFileName(files[0])
			if !ok {
				done <- &fileWaitResult{fmt.Errorf("could not parse filename %s", files[0])}
				return
			}

			if partial {
				waitForBlockNum = start
				if start == moduleStartBlock {
					return
				}
				continue
			}

			return //we are done because we found a kv file.
		}
	}()

	return done
}

func pathToState(ctx context.Context, store *Store, requestStartBlock uint64, moduleStartBlock uint64) ([]string, error) {
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

		prefix := store.StateFilePrefix(nextBlockNum)
		files, err := store.ListFiles(ctx, prefix, "", 2)
		if err != nil {
			return nil, fmt.Errorf("listing file with prefix %s, : %w", prefix, err)
		}

		found := len(files) >= 1
		if !found {
			return nil, fmt.Errorf("file not found to prefix %s for module %s", prefix, store.Name)
		}

		foundFile := files[0]
		switch len(files) {
		case 2:
			_, _, _, partialLeft := parseFileName(files[0])
			_, _, _, partialRight := parseFileName(files[1])
			if partialLeft && partialRight {
				return nil, fmt.Errorf("found two partial files %s and %s", files[0], files[1])
			}
			if !partialRight {
				foundFile = files[1]
			}
		}

		out = append(out, foundFile)

		ok, start, _, partial := parseFileName(foundFile)
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

func init() {
	fullKVRegex = regexp.MustCompile(`[\w]+-([\d]+)\.kv`)
	partialKVRegex = regexp.MustCompile(`[\w]+-([\d]+)-([\d]+)\.partial`)
}

func parseFileName(filename string) (ok bool, start, end uint64, partial bool) {
	if strings.HasSuffix(filename, ".kv") {
		res := fullKVRegex.FindAllStringSubmatch(filename, 1)
		if len(res) != 1 {
			return
		}
		start = 0
		end = uint64(mustAtoi(res[0][1]))
		partial = false
		ok = true
	} else if strings.HasSuffix(filename, ".partial") {
		res := partialKVRegex.FindAllStringSubmatch(filename, 1)
		if len(res) != 1 {
			return
		}
		end = uint64(mustAtoi(res[0][1]))
		start = uint64(mustAtoi(res[0][2]))
		partial = true
		ok = true
	}

	return
}

func mustAtoi(s string) int {
	i, err := strconv.Atoi(s)
	if err != nil {
		panic(err)
	}
	return i
}
