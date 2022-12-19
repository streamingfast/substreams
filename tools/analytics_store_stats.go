package tools

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/storage/store"
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

	baseDStore, err := dstore.NewStore(storePath, "zst", "zstd", false)
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
			done := make(chan any)
			defer close(done)

			hash := hex.EncodeToString(hashes.HashModule(pkg.Modules, module, graph))

			go func() {
				for {
					select {
					case <-time.After(10 * time.Second):
						zlog.Debug("still getting store stats", zap.String("module", module.Name), zap.String("hash", hash))
					case <-done:
						return
					}
				}
			}()

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
				hash,
				module.GetKind().(*pbsubstreams.Module_KindStore_).KindStore.UpdatePolicy,
				module.GetKind().(*pbsubstreams.Module_KindStore_).KindStore.ValueType,
				baseDStore,
			)
			if err != nil {
				zlog.Error("creating store config", zap.Error(err))
				return
			}
			storeStats := initializeStoreStats(conf)

			stateStore, fileInfos, err := getStore(ctx, conf)
			if err != nil {
				if errors.Is(err, EmptyStoreError) {
					zlog.Debug("skipping empty store", zap.String("module", module.Name))
					statsStream <- storeStats
					return
				}

				zlog.Error("creating store", zap.Error(err))
				return
			}

			growth, err := fileSizeGrowth(ctx, conf, fileInfos)
			if err != nil {
				zlog.Error("getting file size growth", zap.Error(err))
				return
			}

			latestFile := fileInfos[len(fileInfos)-1]

			var fileSize int64
			fileSize, err = conf.FileSize(ctx, latestFile)
			if err != nil {
				zlog.Error("getting file size", zap.Error(err))
				return
			}

			storeStats.FileInfo = &FileInfo{
				FileBlockRange: block.NewRange(latestFile.StartBlock, latestFile.EndBlock),
				FileName:       latestFile.Filename,
				FileSize:       fileSize,
				FileSizeGrowth: growth,
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
	FileSize       int64        `json:"size_bytes"`
	FileSizeGrowth float64      `json:"size_growth"`
	FileBlockRange *block.Range `json:"block_range"`
}

type KeyStats struct {
	TotalSize   uint64  `json:"total_size_bytes"`
	LargestSize uint64  `json:"largest_size_bytes"`
	AverageSize float64 `json:"average_size_bytes"`
	StdDevSize  float64 `json:"std_dev_size_bytes"`

	Largest string `json:"largest"`
}

type ValueStats struct {
	TotalSize   uint64  `json:"total_size_bytes"`
	LargestSize uint64  `json:"largest_size_bytes"`
	AverageSize float64 `json:"average_size_bytes"`
	StdDevSize  float64 `json:"std_dev_size_bytes"`

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

func getStore(ctx context.Context, conf *store.Config) (store.Store, []*store.FileInfo, error) {
	start := time.Now()
	files, err := conf.ListSnapshotFiles(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("listing snapshot files: %w", err)
	}
	zlog.Debug("listing snapshot files", zap.Duration("duration", time.Now().Sub(start)))

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

	if len(kvFiles) == 0 {
		zlog.Debug("store only contains partial files", zap.String("module", conf.Name()), zap.String("hash", conf.ModuleHash()))
		return nil, nil, EmptyStoreError
	}

	start = time.Now()
	sort.Slice(kvFiles, func(i, j int) bool { //reverse sort
		return kvFiles[i].EndBlock >= kvFiles[j].EndBlock
	})
	zlog.Debug("sorting snapshot files", zap.Duration("duration", time.Now().Sub(start)))

	var latestFiles []*store.FileInfo
	if len(kvFiles) > 5 {
		latestFiles = kvFiles[:5]
	} else {
		latestFiles = kvFiles
	}

	latestFile := kvFiles[0]

	start = time.Now()
	s := conf.NewFullKV(zlog)
	err = s.Load(ctx, latestFile.EndBlock)
	if err != nil {
		return nil, nil, fmt.Errorf("loading store: %w", err)
	}
	zlog.Debug("loading store", zap.Duration("duration", time.Now().Sub(start)))

	return s, latestFiles, nil
}

func calculateStoreStats(stateStore store.Store, stats *StoreStats) error {
	start := time.Now()
	defer func() {
		zlog.Debug("calculating store stats", zap.Duration("duration", time.Now().Sub(start)))
	}()

	keyStats := &KeyStats{}
	valueStats := &ValueStats{}
	stats.KeyStats = keyStats
	stats.ValueStats = valueStats

	keyLens := make([]float64, 0, 1000)
	valueLens := make([]float64, 0, 1000)

	err := stateStore.Iter(func(key string, value []byte) error {
		stats.KeysCount++
		stats.ValueStats.TotalSize += uint64(len(value))
		stats.KeyStats.TotalSize += uint64(len(key))

		keyLens = append(keyLens, float64(len(key)))
		valueLens = append(valueLens, float64(len(value)))

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
		meanKeyLen := float64(stats.KeyStats.TotalSize) / float64(stats.KeysCount)
		keyLenStdDev := stdDev(keyLens, meanKeyLen)
		stats.KeyStats.StdDevSize = keyLenStdDev

		meanValueLen := float64(stats.ValueStats.TotalSize) / float64(stats.KeysCount)
		valueLenStdDev := stdDev(valueLens, meanValueLen)
		stats.ValueStats.StdDevSize = valueLenStdDev

		stats.KeyStats.AverageSize = meanKeyLen
		stats.ValueStats.AverageSize = meanValueLen
	} else {
		stats.KeyStats = nil
		stats.ValueStats = nil
	}

	return nil
}

func stdDev(xs []float64, mean float64) float64 {
	var sum float64
	for _, x := range xs {
		sum += math.Pow(x-mean, 2)
	}
	return math.Sqrt(sum / float64(len(xs)))
}

func fileSizeGrowth(ctx context.Context, conf *store.Config, files []*store.FileInfo) (float64, error) {
	if len(files) < 2 {
		return 0, nil
	}

	firstSize, err := conf.FileSize(ctx, files[0])
	if err != nil {
		return 0, fmt.Errorf("getting file size: %w", err)
	}
	lastSize, err := conf.FileSize(ctx, files[len(files)-1])
	if err != nil {
		return 0, fmt.Errorf("getting file size: %w", err)
	}

	if firstSize == lastSize {
		return 0, nil
	}

	if len(files) == 2 {
		return float64(lastSize - firstSize), nil
	}

	ixs := make([]float64, 0, len(files))
	sizes := make([]float64, 0, len(files))
	for i := 0; i < len(files); i++ {
		switch i {
		case 0:
			ixs = append(ixs, float64(i))
			sizes = append(sizes, float64(firstSize))
			continue
		case len(files) - 1:
			ixs = append(ixs, float64(i))
			sizes = append(sizes, float64(lastSize))
			continue
		}

		file := files[i]
		s, err := conf.FileSize(ctx, file)
		if err != nil {
			return 0, fmt.Errorf("getting file size: %w", err)
		}
		sizes = append(sizes, float64(s))
		ixs = append(ixs, float64(i))
	}

	m, _ := leastSquareRegression(ixs, sizes)

	return -1 * m, nil
}

func leastSquareRegression(xs, ys []float64) (float64, float64) {
	var sumX, sumY, sumXY, sumXX float64
	for i := range xs {
		sumX += xs[i]
		sumY += ys[i]
		sumXY += xs[i] * ys[i]
		sumXX += xs[i] * xs[i]
	}

	n := float64(len(xs))
	m := (n*sumXY - sumX*sumY) / (n*sumXX - sumX*sumX)
	b := (sumY - m*sumX) / n

	return m, b
}
