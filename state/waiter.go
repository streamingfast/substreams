package state

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	pbtransform "github.com/streamingfast/substreams/pb/sf/substreams/transform/v1"

	"github.com/abourget/llerrgroup"
	"github.com/streamingfast/substreams/manifest"
)

const WaiterSleepInterval = 5 * time.Second

type FileWaiter struct {
	ancestorStores    []*pbtransform.Module
	targetBlockNumber uint64
	storeFactory      FactoryInterface
}

func NewFileWaiter(moduleName string, moduleGraph *manifest.ModuleGraph, factory FactoryInterface, targetStartBlock uint64) *FileWaiter {
	w := &FileWaiter{
		ancestorStores:    nil,
		targetBlockNumber: targetStartBlock,
		storeFactory:      factory,
	}

	ancestorStores, _ := moduleGraph.AncestorStoresOf(moduleName) //todo: new the list of parent store.
	w.ancestorStores = ancestorStores

	return w
}

func (p *FileWaiter) Wait(ctx context.Context, manif *pbtransform.Manifest, graph *manifest.ModuleGraph, requestStartBlock uint64) error {
	eg := llerrgroup.New(len(p.ancestorStores))
	for _, ancestor := range p.ancestorStores {
		if eg.Stop() {
			continue // short-circuit the loop if we got an error
		}

		module := ancestor
		eg.Go(func() error {
			return <-p.wait(ctx, requestStartBlock, module, manifest.HashModuleAsString(manif, module, graph))
		})
	}

	if err := eg.Wait(); err != nil {
		return err
	}

	return nil
}

func WaitKV(ctx context.Context, endBlock uint64, storeFactory FactoryInterface, storeName string, moduleHash string) <-chan error {
	done := make(chan error)
	store := storeFactory.New(storeName, moduleHash) //todo: need to use module signature here.

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

			fileName := fmt.Sprintf("%s-%d.kv", storeName, endBlock)
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

func (p *FileWaiter) wait(ctx context.Context, requestStartBlock uint64, module *pbtransform.Module, moduleHash string) <-chan error {
	done := make(chan error)
	store := p.storeFactory.New(module.Name, moduleHash) //todo: need to use module signature here.
	waitForBlockNum := requestStartBlock
	//       END  START
	//module_3000_2000.partial
	//module_2000_1000.partial
	//module_1000.kv
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

			if p.targetBlockNumber <= module.StartBlock {
				return
			}

			prefix := fmt.Sprintf("%s-%d", module.Name, waitForBlockNum)
			files, err := store.ListFiles(ctx, prefix, "", 1)
			if err != nil {
				done <- &fileWaitResult{fmt.Errorf("listing file with prefix %s, : %w", prefix, err)}
				return
			}

			found := len(files) == 1
			if !found {
				time.Sleep(WaiterSleepInterval)
				fmt.Printf("waiting for store %s to complete processing to block %d\n", module.Name, p.targetBlockNumber)
				continue
			}

			ok, start, _, partial := parseFileName(files[0])
			if !ok {
				done <- &fileWaitResult{fmt.Errorf("could not parse filename %s", files[0])}
				return
			}

			//todo: validate that start block match configured kv block range.
			if partial {
				waitForBlockNum = start
				//todo 2.1: if start block from partial is the module start block we are done waiting
				if start == module.GetStartBlock() {
					return
				}
				continue
			}

			return //we are done because we found a kv file.
		}
	}()

	return done
}

func pathToState(ctx context.Context, store Store, requestStartBlock uint64, moduleName string, moduleStartBlock uint64) ([]string, error) {
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

		prefix := fmt.Sprintf("%s-%d", moduleName, nextBlockNum)
		files, err := store.ListFiles(ctx, prefix, "", 2)
		if err != nil {
			return nil, fmt.Errorf("listing file with prefix %s, : %w", prefix, err)
		}

		found := len(files) >= 1
		if !found {
			return nil, fmt.Errorf("file not found to prefix %s for module %s", prefix, moduleName)
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
