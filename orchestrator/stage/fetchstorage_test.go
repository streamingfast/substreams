package stage

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/block"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/storage/execout"
	"github.com/streamingfast/substreams/storage/store"
	"github.com/stretchr/testify/assert"
	"github.com/test-go/testify/require"
	"go.uber.org/zap"
)

var testLogger, _ = zap.NewDevelopment()
var storePreWalked = cmap.New[bool]()

func TestFetchOutputMapperState(t *testing.T) {
	tests := []struct {
		name          string
		stages        *Stages
		startBlockNum uint64
		wantStates    string
		wantErr       bool
	}{
		{
			name: "invalid block range returns empty",
			stages: &Stages{
				globalSegmenter: segmenter(100, 99),
				mapSegmenter:    segmenter(100, 99),
				stages: []*Stage{
					{
						storeModuleStates: []*StoreModuleState{
							{name: "test_mapper"},
						},
					},
				},
				execoutConfigs: execoutConfigs(t, map[string][]string{"test_mapper:0": {
					"hash/outputs/0000000000-000000010.output",
				}}),
			},
			wantStates: "M:\n",
			wantErr:    false,
		},
		{
			name: "single output mapper",
			stages: &Stages{
				globalSegmenter: segmenter(0, 100),
				mapSegmenter:    segmenter(0, 100),
				stages: []*Stage{
					{
						segmenter: segmenter(0, 100),
						storeModuleStates: []*StoreModuleState{
							{name: "test_mapper"},
						},
					},
				},
				execoutConfigs: execoutConfigs(t, map[string][]string{"test_mapper:0": {
					"hash/outputs/0000000000-000000010.output",
					"hash/outputs/0000000010-000000020.output",
				}}),
			},
			wantStates: `M:CC`,
			wantErr:    false,
		},
		{
			name: "mapper fetched optimistically",
			stages: &Stages{
				globalSegmenter: segmenter(60, 100),
				mapSegmenter:    segmenter(60, 100),
				stages: []*Stage{
					{
						segmenter: segmenter(0, 100),
						storeModuleStates: []*StoreModuleState{
							{name: "test_mapper"},
						},
					},
				},
				execoutConfigs: execout.WrapConfigs(10, testLogger, testExecoutConfig(t, "test_mapper", 0, "hash", &dstore.MockStore{
					WalkFunc:     failWalk(t),
					WalkFromFunc: failWalkFrom(t),
					WalkFromToFunc: func(ctx context.Context, prefix, start, end string, f func(string) error) error {
						_, found := storePreWalked.Get("test_store")
						if !found {
							storePreWalked.Set("test_store", true)
							assert.Equal(t, "0000000060-0000000000.output", start)
							assert.Equal(t, "0000000099-0000000000.output", end) // ends at 99 because end block is exclusive
							return nil
						}
						t.Fatal("should not call walk on mapper twice")
						return fmt.Errorf("test failed")
					},
				})),
			},
			wantStates: `M:`,
			wantErr:    false,
		},

		{
			name: "mapper and store, misaligned",
			stages: &Stages{
				globalSegmenter: segmenter(0, 100),
				mapSegmenter:    segmenter(15, 100),
				storeSegmenter:  segmenter(0, 100),
				stages: []*Stage{
					{
						segmenter: segmenter(0, 100),
						storeModuleStates: []*StoreModuleState{
							{
								name:      "test_store",
								segmenter: segmenter(0, 100),
							},
						},
						kind: KindStore,
					},
					{
						segmenter: segmenter(0, 100),
						storeModuleStates: []*StoreModuleState{
							{
								name:      "test_mapper",
								segmenter: segmenter(0, 100),
							},
						},
						kind: KindMap,
					},
				},

				execoutConfigs: execoutConfigs(t, map[string][]string{"test_mapper:0": {
					"hash/outputs/0000000015-000000020.output",
					"hash/outputs/0000000020-000000030.output",
				}}),
				storeConfigs: store.ConfigMap{
					"test_store": testStoreConfig(t, "test_store", 0, "hash", pbsubstreams.Module_KindStore_UPDATE_POLICY_APPEND, "value", &dstore.MockStore{
						Files: map[string][]byte{
							"hash/states/0000000010-0000000000.kv": nil,
							"hash/states/0000000020-0000000000.kv": nil,
							"hash/states/0000000030-0000000000.kv": nil,
						}},
					),
				},
			},
			wantStates: `S:CCC
			M:.CC`,
			wantErr: false,
		},

		{
			name:          "stores not found in first pass, fetches all",
			startBlockNum: 60, // dev mode needs stores ready at 60
			stages: &Stages{
				globalSegmenter: segmenter(0, 60), // calculated from the storeModuleStates
				storeSegmenter:  segmenter(0, 60), // calculated from the storeModuleStates
				stages: []*Stage{
					{
						segmenter: segmenter(0, 80), // not sure how this one is used
						storeModuleStates: []*StoreModuleState{
							{
								name:      "test_store",
								segmenter: segmenter(0, 60),
							},
							{
								name:      "test_store2",
								segmenter: segmenter(10, 60),
							},
						},
						kind: KindStore,
					},
				},
				storeConfigs: store.ConfigMap{
					"test_store": testStoreConfig(t, "test_store", 0, "hash", pbsubstreams.Module_KindStore_UPDATE_POLICY_APPEND, "value", &dstore.MockStore{
						WalkFunc:     failWalk(t),
						WalkFromFunc: failWalkFrom(t),
						WalkFromToFunc: func(ctx context.Context, prefix, start, end string, f func(string) error) error {
							_, found := storePreWalked.Get("test_store")
							if !found {
								storePreWalked.Set("test_store", true)
								assert.Equal(t, "0000000030-", start)
								assert.Equal(t, "0000000061-", end)
								return nil
							}

							assert.Equal(t, "0000000000-", start)
							assert.Equal(t, "0000000061-", end)
							return walkFiles([]string{
								"hash/states/0000000010-0000000000.kv",
								"hash/states/0000000020-0000000000.kv",
							}, f)
						},
					}),

					"test_store2": testStoreConfig(t, "test_store2", 10, "hash", pbsubstreams.Module_KindStore_UPDATE_POLICY_APPEND, "value", &dstore.MockStore{
						WalkFunc:     failWalk(t),
						WalkFromFunc: failWalkFrom(t),
						WalkFromToFunc: func(ctx context.Context, prefix, start, end string, f func(string) error) error {
							_, found := storePreWalked.Get("test_store2")
							if !found {
								storePreWalked.Set("test_store2", true)
								assert.Equal(t, "0000000030-", start)
								assert.Equal(t, "0000000061-", end)
								return nil
							}

							assert.Equal(t, "0000000000-", start)
							assert.Equal(t, "0000000061-", end)
							return walkFiles([]string{
								"hash/states/0000000020-0000000010.kv",
							}, f)
						},
					}),
				},
			},
			wantStates: `S:CC`, // segments [0-10], [10-20]
			wantErr:    false,
		},
		{
			name:          "many stores on same stage, needs 1",
			startBlockNum: 40,
			stages: &Stages{
				globalSegmenter: segmenter(0, 40), // calculated from the storeModuleStates
				storeSegmenter:  segmenter(0, 40), // calculated from the storeModuleStates
				stages: []*Stage{
					{
						segmenter: segmenter(0, 80), // not sure how this one is used
						storeModuleStates: []*StoreModuleState{
							{
								name:      "test_store",
								segmenter: segmenter(0, 40),
							},
							{
								name:      "test_store2",
								segmenter: segmenter(40, 40),
							},
							{
								name:      "test_store3",
								segmenter: segmenter(80, 40),
							},
						},
						kind: KindStore,
					},
				},
				storeConfigs: store.ConfigMap{
					"test_store": testStoreConfig(t, "test_store", 0, "hash", pbsubstreams.Module_KindStore_UPDATE_POLICY_APPEND, "value", &dstore.MockStore{
						WalkFunc:     failWalk(t),
						WalkFromFunc: failWalkFrom(t),
						WalkFromToFunc: func(ctx context.Context, prefix, start, end string, f func(string) error) error {
							assert.Equal(t, "0000000010-", start)
							assert.Equal(t, "0000000041-", end)
							return walkFiles([]string{
								"hash/states/0000000010-0000000000.kv",
								"hash/states/0000000020-0000000000.kv",
								"hash/states/0000000030-0000000000.kv",
							}, f)
						},
					}),

					"test_store2": testStoreConfig(t, "test_store2", 0, "hash", pbsubstreams.Module_KindStore_UPDATE_POLICY_APPEND, "value", &dstore.MockStore{
						WalkFunc:     failWalk(t),
						WalkFromFunc: failWalkFrom(t),
						WalkFromToFunc: func(ctx context.Context, prefix, start, end string, f func(string) error) error {
							assert.Equal(t, "0000000010-", start)
							assert.Equal(t, "0000000041-", end)
							return nil
						},
					}),

					"test_store3": testStoreConfig(t, "test_store3", 0, "hash", pbsubstreams.Module_KindStore_UPDATE_POLICY_APPEND, "value", &dstore.MockStore{
						WalkFunc:     failWalk(t),
						WalkFromFunc: failWalkFrom(t),
						WalkFromToFunc: func(ctx context.Context, prefix, start, end string, f func(string) error) error {
							assert.Equal(t, "0000000010-", start)
							assert.Equal(t, "0000000041-", end)
							return nil
						},
					}),
				},
			},
			wantStates: `S:CCC`, // segments [0-10], [10-20] and [20-30] are fetched
			wantErr:    false,
		},
		{
			name:          "many stores on same stage, needs 2",
			startBlockNum: 55,
			stages: &Stages{
				globalSegmenter: segmenter(0, 50), // calculated from the storeModuleStates
				storeSegmenter:  segmenter(0, 50), // calculated from the storeModuleStates
				stages: []*Stage{
					{
						segmenter: segmenter(0, 80), // not sure how this one is used
						storeModuleStates: []*StoreModuleState{
							{
								name:      "test_store",
								segmenter: segmenter(0, 50),
							},
							{
								name:      "test_store2",
								segmenter: segmenter(40, 50),
							},
							{
								name:      "test_store3",
								segmenter: segmenter(80, 50),
							},
						},
						kind: KindStore,
					},
				},
				storeConfigs: store.ConfigMap{
					"test_store": testStoreConfig(t, "test_store", 0, "hash", pbsubstreams.Module_KindStore_UPDATE_POLICY_APPEND, "value", &dstore.MockStore{
						WalkFunc:     failWalk(t),
						WalkFromFunc: failWalkFrom(t),
						WalkFromToFunc: func(ctx context.Context, prefix, start, end string, f func(string) error) error {
							assert.Equal(t, "0000000020-", start)
							assert.Equal(t, "0000000051-", end)
							return walkFiles([]string{
								// 0-10 is inferred
								"hash/states/0000000020-0000000000.kv",
								"hash/states/0000000030-0000000000.kv",
								"hash/states/0000000040-0000000000.kv",
								"hash/states/0000000050-0000000000.kv",
							}, f)
						},
					}),

					"test_store2": testStoreConfig(t, "test_store2", 0, "hash", pbsubstreams.Module_KindStore_UPDATE_POLICY_APPEND, "value", &dstore.MockStore{
						WalkFunc:     failWalk(t),
						WalkFromFunc: failWalkFrom(t),
						WalkFromToFunc: func(ctx context.Context, prefix, start, end string, f func(string) error) error {
							assert.Equal(t, "0000000020-", start)
							assert.Equal(t, "0000000051-", end)
							return walkFiles([]string{
								"hash/states/0000000050-0000000040.kv",
							}, f)
						},
					}),

					"test_store3": testStoreConfig(t, "test_store3", 0, "hash", pbsubstreams.Module_KindStore_UPDATE_POLICY_APPEND, "value", &dstore.MockStore{
						WalkFunc:     failWalk(t),
						WalkFromFunc: failWalkFrom(t),
						WalkFromToFunc: func(ctx context.Context, prefix, start, end string, f func(string) error) error {
							assert.Equal(t, "0000000020-", start)
							assert.Equal(t, "0000000051-", end)
							return nil
						},
					}),
				},
			},
			wantStates: `S:CCCCC`, // segments [0-10], [10-20] and [20-30], [30-40], [40-50] are fetched
			wantErr:    false,
		},

		{
			name:          "mapper on production mode is sufficient on range 55-60",
			startBlockNum: 55,
			stages: &Stages{
				globalSegmenter: segmenter(0, 60),
				storeSegmenter:  segmenter(0, 50),  // calculated from the storeModuleStates
				mapSegmenter:    segmenter(50, 60), // this infers "production-mode"
				stages: []*Stage{
					{
						segmenter: segmenter(0, 50), // not sure how this one is used
						storeModuleStates: []*StoreModuleState{
							{
								name:      "test_store",
								segmenter: segmenter(0, 50),
							},
						},
						kind: KindStore,
					},
					{
						segmenter: segmenter(50, 60), // not sure how this one is used
						storeModuleStates: []*StoreModuleState{
							{
								name:      "test_mapper",
								segmenter: segmenter(50, 60),
							},
						},
						kind: KindMap,
					},
				},
				storeConfigs: store.ConfigMap{
					"test_store": testStoreConfig(t, "test_store", 0, "hash", pbsubstreams.Module_KindStore_UPDATE_POLICY_APPEND, "value", &dstore.MockStore{ // store must NOT be called
						WalkFunc:       failWalk(t),
						WalkFromFunc:   failWalkFrom(t),
						WalkFromToFunc: failWalkFromTo(t),
					}),
				},

				execoutConfigs: execoutConfigs(t, map[string][]string{"test_mapper:0": {
					"hash/outputs/0000000050-000000060.output",
				}}),
			},
			wantStates: `S:NNNNN.
			M:.....C`,
			wantErr: false,
		},
		{
			name:          "mapper on production mode still needs stores if there is linear after",
			startBlockNum: 55, // open range, linear pipeline would start at 60
			stages: &Stages{
				hasLinearPipeline: true,
				globalSegmenter:   segmenter(0, 60),
				storeSegmenter:    segmenter(0, 60),  // calculated from the storeModuleStates
				mapSegmenter:      segmenter(50, 60), // this infers "production-mode"
				stages: []*Stage{
					{
						segmenter: segmenter(0, 60), // not sure how this one is used
						storeModuleStates: []*StoreModuleState{
							{
								name:      "test_store",
								segmenter: segmenter(0, 60),
							},
						},
						kind: KindStore,
					},
					{
						segmenter: segmenter(50, 60), // not sure how this one is used
						storeModuleStates: []*StoreModuleState{
							{
								name:      "test_mapper",
								segmenter: segmenter(50, 60),
							},
						},
						kind: KindMap,
					},
				},
				storeConfigs: store.ConfigMap{
					"test_store": testStoreConfig(t, "test_store", 0, "hash", pbsubstreams.Module_KindStore_UPDATE_POLICY_APPEND, "value", &dstore.MockStore{ // store must NOT be called
						WalkFunc:     failWalk(t),
						WalkFromFunc: failWalkFrom(t),
						WalkFromToFunc: func(ctx context.Context, prefix, start, end string, f func(string) error) error {
							assert.Equal(t, "0000000030-", start)
							assert.Equal(t, "0000000061-", end)
							return walkFiles([]string{
								"hash/states/0000000060-0000000000.kv", // this one is sufficient
							}, f)
						},
					}),
				},

				execoutConfigs: execoutConfigs(t, map[string][]string{"test_mapper:0": {
					"hash/outputs/0000000050-000000060.output",
				}}),
			},
			wantStates: `S:CCCCCC
					M:.....C`,
			wantErr: false,
		},

		{
			name:          "mapper on production mode may fallback to stores",
			startBlockNum: 55,
			stages: &Stages{
				globalSegmenter: segmenter(0, 70),
				storeSegmenter:  segmenter(0, 50),  // calculated from the storeModuleStates
				mapSegmenter:    segmenter(50, 70), // this infers "production-mode"
				stages: []*Stage{
					{
						segmenter: segmenter(0, 50), // not sure how this one is used
						storeModuleStates: []*StoreModuleState{
							{
								name:      "test_store",
								segmenter: segmenter(0, 50),
							},
						},
						kind: KindStore,
					},
					{
						segmenter: segmenter(50, 70), // not sure how this one is used
						storeModuleStates: []*StoreModuleState{
							{
								name:      "test_mapper",
								segmenter: segmenter(50, 70),
							},
						},
						kind: KindMap,
					},
				},
				storeConfigs: store.ConfigMap{
					"test_store": testStoreConfig(t, "test_store", 0, "hash", pbsubstreams.Module_KindStore_UPDATE_POLICY_APPEND, "value", &dstore.MockStore{ // store must NOT be called
						WalkFunc:     failWalk(t),
						WalkFromFunc: failWalkFrom(t),
						WalkFromToFunc: func(ctx context.Context, prefix, start, end string, f func(string) error) error {
							assert.Equal(t, "0000000020-", start)
							assert.Equal(t, "0000000051-", end)
							return walkFiles([]string{
								// 0-10 is inferred
								"hash/states/0000000020-0000000000.kv",
								"hash/states/0000000030-0000000000.kv",
								"hash/states/0000000040-0000000000.kv",
							}, f)
						},
					}),
				},

				execoutConfigs: execoutConfigs(t, map[string][]string{"test_mapper:0": {
					"hash/outputs/0000000050-000000060.output",
				}}),
			},
			wantStates: `S:CCCC..
			M:.....C`,
			wantErr: false,
		},
	}

	firstPassStoresWalkMaxSegments = 2 // for tests
	for _, tt := range tests {

		storePreWalked = cmap.New[bool]() // reset this every pass
		t.Run(tt.name, func(t *testing.T) {
			ctx := reqctx.WithRequest(context.TODO(), &reqctx.RequestDetails{
				ResolvedStartBlockNum: tt.startBlockNum,
			})

			err := tt.stages.FetchCachesState(ctx)

			if (err != nil) != tt.wantErr {
				t.Errorf("fetchOutputMapperState() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			segmentStateEquals(t, tt.stages, tt.wantStates)
		})
	}
}

func segmenter(start, end uint64) *block.Segmenter {
	return block.NewSegmenter(10, start, end)
}

func execoutConfigs(t *testing.T, mappers map[string][]string) *execout.Configs {
	var configs []*execout.Config
	for mapper, files := range mappers {
		splitName := strings.Split(mapper, ":")
		if len(splitName) != 2 {
			panic("invalid mapper name, expecting format <name>:<initialBlock>")
		}
		initialBlock, err := strconv.ParseUint(splitName[1], 10, 64)
		if err != nil {
			panic("invalid mapper name, expecting format <name>:<initialBlock>")
		}

		filesMap := make(map[string][]byte)
		for _, file := range files {
			filesMap[file] = nil
		}
		store := &dstore.MockStore{
			Files: filesMap,
		}

		conf, err := execout.NewConfig(splitName[0], initialBlock, pbsubstreams.ModuleKindMap, "hash", "ext_hash", store, testLogger)
		require.NoError(t, err)
		configs = append(configs, conf)
	}
	return execout.WrapConfigs(10, testLogger, configs...)
}

func testStoreConfig(
	t *testing.T,
	name string,
	moduleInitialBlock uint64,
	moduleHash string,
	updatePolicy pbsubstreams.Module_KindStore_UpdatePolicy,
	valueType string,
	objStore dstore.Store,
) *store.Config {
	conf, err := store.NewConfig(name, moduleInitialBlock, moduleHash, updatePolicy, valueType, objStore, nil, "")
	require.NoError(t, err)
	return conf
}

func testExecoutConfig(t *testing.T, name string, moduleInitialBlock uint64, moduleHash string, store dstore.Store) *execout.Config {
	conf, err := execout.NewConfig(name, moduleInitialBlock, pbsubstreams.ModuleKindMap, moduleHash, "ext_"+moduleHash, store, testLogger)
	require.NoError(t, err)
	return conf
}

func mapFileInfo(start, end uint64) *execout.FileInfo {
	return &execout.FileInfo{
		Filename:   fmt.Sprintf("%010d-%010d.output", start, end),
		BlockRange: &block.Range{StartBlock: start, ExclusiveEndBlock: end},
	}
}

func walkFiles(files []string, f func(string) error) error {
	for _, file := range files {
		if err := f(file); err != nil {
			return err
		}
	}
	return nil
}

// NOTE: we cannot use Panic() or t.Fatal() in this function, since it runs inside `eg.Go()` and handling panicking functions is handled badly
func failWalk(t *testing.T) func(context.Context, string, func(string) error) error {
	return func(_ context.Context, _ string, f func(string) error) error {
		t.Error("failWalk should not be called")
		return fmt.Errorf("test failed")
	}
}

func failWalkFrom(t *testing.T) func(context.Context, string, string, func(string) error) error {
	return func(_ context.Context, _ string, _ string, f func(string) error) error {
		t.Error("failWalkFrom should not be called")
		return fmt.Errorf("test failed")
	}
}

func failWalkFromTo(t *testing.T) func(context.Context, string, string, string, func(string) error) error {
	return func(_ context.Context, _ string, _ string, _ string, f func(string) error) error {
		t.Error("failWalkFromTo should not be called")
		return fmt.Errorf("test failed")
	}
}
