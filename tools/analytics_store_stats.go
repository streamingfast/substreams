package tools

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/store"
	"go.uber.org/zap"
)

var analyticsStoreStatsCmd = &cobra.Command{
	Use:   "store-stats <manifest> <store>",
	Short: "Prints stats about a store",
	Args:  cobra.ExactArgs(2),
	RunE:  StoreStatsE,
}

func init() {
	analyticsCmd.AddCommand(analyticsStoreStatsCmd)
}

var EmptyStoreError = errors.New("store is empty")

func StoreStatsE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	manifestPath := args[0]
	storePath := args[1]

	baseDStore, err := dstore.NewStore(storePath, "", "", false)
	if err != nil {
		return fmt.Errorf("creating base store: %w", err)
	}

	manifestReader := manifest.NewReader(manifestPath)
	pkg, err := manifestReader.Read()
	if err != nil {
		return fmt.Errorf("read manifest %q: %w", manifestPath, err)
	}

	graph, err := manifest.NewModuleGraph(pkg.Modules.Modules)
	if err != nil {
		return fmt.Errorf("creating module graph: %w", err)
	}

	wg := sync.WaitGroup{}
	wg.Add(len(pkg.Modules.Modules))

	stats := make([]*StoreStats, 0, len(pkg.Modules.Modules))
	statsStream := make(chan *StoreStats)

	go func() {
		start := time.Now()
		wg.Wait()
		zlog.Debug("finished getting store stats", zap.Duration("duration", time.Now().Sub(start)))
		close(statsStream)
	}()

	hashes := manifest.NewModuleHashes()
	for _, module := range pkg.Modules.Modules {
		go func(module *pbsubstreams.Module) {
			start := time.Now()
			defer func() {
				zlog.Debug("finished getting store stats for module", zap.String("module", module.Name), zap.Duration("duration", time.Now().Sub(start)))
			}()

			defer wg.Done()
			if module.GetKindStore() == nil {
				zlog.Debug("skipping non-store module", zap.String("module", module.Name))
				return
			}

			conf, err := store.NewConfig(
				module.Name,
				module.InitialBlock,
				hex.EncodeToString(hashes.HashModule(pkg.Modules, module, graph)),
				module.GetKind().(*pbsubstreams.Module_KindStore_).KindStore.UpdatePolicy,
				module.GetKind().(*pbsubstreams.Module_KindStore_).KindStore.ValueType,
				baseDStore,
			)
			if err != nil {
				zlog.Error("creating store config", zap.Error(err))
				return
			}
			storeStats := initializeStoreStats(conf)

			stateStore, fileInfo, err := getStore(ctx, conf)
			if err != nil {
				if errors.Is(err, EmptyStoreError) {
					zlog.Debug("skipping empty store", zap.String("module", module.Name))
					statsStream <- storeStats
					return
				}

				zlog.Error("creating store", zap.Error(err))
				return
			}

			var fileSize uint64
			fileSize, err = conf.FileSize(ctx, fileInfo)
			if err != nil {
				zlog.Error("getting file size", zap.Error(err))
				return
			}
			storeStats.FileInfo = &FileInfo{
				FileBlockRange: block.NewRange(fileInfo.StartBlock, fileInfo.EndBlock),
				FileName:       fileInfo.Filename,
				FileSize:       fileSize,
			}

			err = calculateStoreStats(stateStore, storeStats)
			if err != nil {
				zlog.Error("getting store stats", zap.Error(err))
				return
			}

			statsStream <- storeStats
			return
		}(module)
	}

	for stat := range statsStream {
		stats = append(stats, stat)
	}

	//sort the modules for consistent output
	sortedModules, _ := graph.TopologicalSort()
	sortedModulesIndex := make(map[string]int, len(sortedModules))
	for i, module := range sortedModules {
		sortedModulesIndex[module.Name] = i
	}
	sort.Slice(stats, func(i, j int) bool {
		return sortedModulesIndex[stats[i].Name] > sortedModulesIndex[stats[j].Name]
	})

	data, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling stats to json: %w", err)
	}

	_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(data))
	return nil
}

type StoreStats struct {
	Name         string `json:"module_name"`
	ModuleHash   string `json:"module_hash"`
	InitialBlock uint64 `json:"module_initial_block"`
	ValueType    string `json:"module_value_type"`
	UpdatePolicy string `json:"module_update_policy"`

	KeysCount uint64 `json:"count"`

	FileInfo   *FileInfo   `json:"file_info,omitempty"`
	KeyStats   *KeyStats   `json:"keys,inline,omitempty"`
	ValueStats *ValueStats `json:"values,inline,omitempty"`
}

type FileInfo struct {
	FileName       string       `json:"name"`
	FileSize       uint64       `json:"size_bytes"`
	FileBlockRange *block.Range `json:"block_range"`
}

type KeyStats struct {
	TotalSize   uint64  `json:"total_size_bytes"`
	LargestSize uint64  `json:"largest_size_bytes"`
	AverageSize float64 `json:"average_size_bytes"`

	Largest string `json:"largest"`
}

type ValueStats struct {
	TotalSize   uint64  `json:"total_size_bytes"`
	LargestSize uint64  `json:"largest_size_bytes"`
	AverageSize float64 `json:"average_size_bytes"`

	Largest string `json:"largest_value_key"`
}

func initializeStoreStats(conf *store.Config) *StoreStats {
	storeStats := &StoreStats{
		Name:         conf.Name(),
		ModuleHash:   conf.ModuleHash(),
		ValueType:    conf.ValueType(),
		UpdatePolicy: conf.UpdatePolicy().String(),
		InitialBlock: conf.ModuleInitialBlock(),
	}

	return storeStats
}

func getStore(ctx context.Context, conf *store.Config) (store.Store, *store.FileInfo, error) {
	files, err := conf.ListSnapshotFiles(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("listing snapshot files: %w", err)
	}
	if len(files) == 0 {
		zlog.Debug("store is empty", zap.String("module", conf.Name()), zap.String("hash", conf.ModuleHash()))
		return nil, nil, EmptyStoreError
	}

	kvFiles := make([]*store.FileInfo, 0, len(files))
	for _, file := range files {
		if file.Partial {
			continue
		}
		kvFiles = append(kvFiles, file)
	}

	sort.Slice(kvFiles, func(i, j int) bool { //reverse sort
		return kvFiles[i].EndBlock >= kvFiles[j].EndBlock
	})
	latestFile := kvFiles[0]

	s := conf.NewFullKV(zlog)
	err = s.Load(ctx, latestFile.EndBlock)
	if err != nil {
		return nil, nil, fmt.Errorf("loading store: %w", err)
	}

	return s, latestFile, nil
}

func calculateStoreStats(stateStore store.Store, stats *StoreStats) error {
	keyStats := &KeyStats{}
	valueStats := &ValueStats{}
	stats.KeyStats = keyStats
	stats.ValueStats = valueStats

	err := stateStore.Iter(func(key string, value []byte) error {
		stats.KeysCount++
		stats.ValueStats.TotalSize += uint64(len(value))
		stats.KeyStats.TotalSize += uint64(len(key))

		if uint64(len(key)) > stats.KeyStats.LargestSize {
			stats.KeyStats.LargestSize = uint64(len(key))
			stats.KeyStats.Largest = key
		}

		if uint64(len(value)) > stats.ValueStats.LargestSize {
			stats.ValueStats.LargestSize = uint64(len(value))
			stats.ValueStats.Largest = key
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("iterating store: %w", err)
	}

	if stats.KeysCount > 0 {
		stats.KeyStats.AverageSize = float64(stats.KeyStats.TotalSize) / float64(stats.KeysCount)
		stats.ValueStats.AverageSize = float64(stats.ValueStats.TotalSize) / float64(stats.KeysCount)
	} else {
		stats.KeyStats = nil
		stats.ValueStats = nil
	}

	return nil
}
