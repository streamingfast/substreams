package state

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/abourget/llerrgroup"
	"github.com/streamingfast/substreams/manifest"
	"github.com/yourbasic/graph"
)

type node struct {
	Name       string
	StartBlock uint64
	Store      StoreInterface
}

const WaiterSleepInterval = 5 * time.Second

type FileWaiter struct {
	ancestorStores         []*node
	targetStartBlockNumber uint64
}

func NewFileWaiter(moduleName string, moduleGraph *manifest.ModuleGraph, factory FactoryInterface, targetStartBlock uint64) *FileWaiter {
	w := &FileWaiter{
		ancestorStores:         nil,
		targetStartBlockNumber: targetStartBlock,
	}

	ancestorStores, _ := moduleGraph.AncestorStoresOf(moduleName)

	var parentNodes []*node
	for _, ancestorStore := range ancestorStores {
		partialNode := &node{
			Name:       ancestorStore.Name,
			Store:      factory.New(ancestorStore.Name),
			StartBlock: ancestorStore.InitialBlock,
		}
		parentNodes = append(parentNodes, partialNode)
	}
	w.ancestorStores = parentNodes

	return w
}

func (p *FileWaiter) Wait(ctx context.Context) error {
	eg := llerrgroup.New(len(p.ancestorStores))
	for _, parent := range p.ancestorStores {
		node := parent
		eg.Go(func() error {
			return <-p.wait(ctx, node)
		})
	}

	if err := eg.Wait(); err != nil {
		return err
	}

	return nil
}

func (p *FileWaiter) wait(ctx context.Context, node *node) <-chan error {
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

			exists, _, err := ContiguousFilesToTargetBlock(ctx, node.Name, node.Store, node.StartBlock, p.targetStartBlockNumber)
			if err != nil {
				done <- &fileWaitResult{ctx.Err()}
				return
			}

			if exists {
				return
			}

			time.Sleep(WaiterSleepInterval)
			fmt.Printf("waiting for store %s to complete processing to block %d\n", node.Name, p.targetStartBlockNumber)
		}
	}()

	return done
}

func ContiguousFilesToTargetBlock(ctx context.Context, storeName string, store StoreInterface, startBlock, targetBlock uint64) (bool, []string, error) {
	/// walk files and create a graph where edges link the end-block of one file to the start-block of another.
	/// this way, we can know if a store has all the necessary files to cover all the data up to the target block by checking if there is a path in
	///   the graph to the target

	//get all block ranges for this store
	var ranges blockRangeItems
	err := store.Walk(ctx, fmt.Sprintf("%s-", storeName), ".tmp", func(filename string) error {
		ok, start, end := parseFileName(filename)
		if !ok {
			return fmt.Errorf("could not parse filename %s", filename)
		}

		bri := blockRangeItem{
			start:    start,
			end:      end,
			filename: filename,
		}
		ranges = append(ranges, bri)

		return nil
	})

	if err != nil {
		return false, nil, fmt.Errorf("error walking files: %w", err)
	}

	sort.Sort(ranges)

	fulls := map[int]struct{}{}
	targets := map[int]struct{}{}
	for i, x := range ranges {
		if x.start == 0 || x.start == startBlock {
			fulls[i] = struct{}{}
		}
		if x.end == targetBlock {
			targets[i] = struct{}{}
		}
	}

	if len(fulls) == 0 {
		return false, nil, nil // no files which start at the beginning
	}

	if len(targets) == 0 { // no files which reach the target block
		return false, nil, nil
	}

	var ends []int
	for _, br := range ranges {
		ends = append(ends, int(br.end))
	}

	// construct a graph with all the paths of ranges
	g := graph.New(len(ranges))
	for i, e := range ends {
		for j, br := range ranges {
			if uint64(e) == br.start {
				g.AddCost(i, j, 1)
			}
		}
	}

	//check if there is a path from any of the full snapshots (start = 0) to our target block
	var path []int
	for t := range targets {
		for f := range fulls {
			p, _ := graph.ShortestPath(g, f, t)
			if len(p) > 0 {
				path = p
				break
			}
		}
	}

	if len(path) == 0 {
		return false, nil, nil
	}

	var pathFileNames []string
	for _, p := range path {
		pathFileNames = append(pathFileNames, ranges[p].filename)
	}

	return true, pathFileNames, nil
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

func parseFileName(filename string) (ok bool, start, end uint64) {
	if strings.HasSuffix(filename, ".kv") {
		res := fullKVRegex.FindAllStringSubmatch(filename, 1)
		if len(res) != 1 {
			return
		}
		start = 0
		end = uint64(mustAtoi(res[0][1]))
		ok = true
	} else if strings.HasSuffix(filename, ".partial") {
		res := partialKVRegex.FindAllStringSubmatch(filename, 1)
		if len(res) != 1 {
			return
		}
		start = uint64(mustAtoi(res[0][1]))
		end = uint64(mustAtoi(res[0][2]))
		ok = true
	}

	return
}

type blockRangeItem struct {
	start uint64
	end   uint64

	filename string
}

type blockRangeItems []blockRangeItem

func (b blockRangeItems) Len() int {
	return len(b)
}

func (b blockRangeItems) Less(i, j int) bool {
	if b[i].start == b[j].start {
		return b[i].end < b[j].end
	}
	return b[i].end < b[j].start
}

func (b blockRangeItems) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}

func mustAtoi(s string) int {
	i, err := strconv.Atoi(s)
	if err != nil {
		panic(err)
	}
	return i
}
