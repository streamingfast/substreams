package integration

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	pbindexes "github.com/streamingfast/substreams/storage/index/pb"
	"google.golang.org/protobuf/proto"

	"github.com/streamingfast/substreams/storage/index"

	"github.com/RoaringBitmap/roaring/roaring64"

	"github.com/streamingfast/dstore"

	pboutput "github.com/streamingfast/substreams/storage/execout/pb"

	"github.com/streamingfast/substreams/manifest"

	"github.com/streamingfast/substreams/block"
	"github.com/stretchr/testify/require"

	"github.com/streamingfast/substreams/orchestrator/work"
	"github.com/streamingfast/substreams/reqctx"
)

type preCreatedIndices struct {
	fileName string
	indices  map[string]*roaring64.Bitmap
}

func TestTier2Call(t *testing.T) {
	manifest.UseSimpleHash = true
	mapInit50 := hex.EncodeToString([]byte("map_output_init_50"))
	secondMapInit50 := hex.EncodeToString([]byte("second_map_output_init_50"))

	firstStoreInit20 := hex.EncodeToString([]byte("first_store_init_20"))
	secondStoreInit30 := hex.EncodeToString([]byte("second_store_init_30"))
	thirdStoreInit40 := hex.EncodeToString([]byte("third_store_init_40"))
	fourthStoreInit52 := hex.EncodeToString([]byte("fourth_store_init_52"))
	blockIndexInit60 := hex.EncodeToString([]byte("index_init_60"))
	mapUsingIndexInit70 := hex.EncodeToString([]byte("map_using_index_init_70"))

	randomIndicesRange := roaring64.New()
	randomIndicesRange.AddInt(70)
	randomIndicesRange.AddInt(71)
	randomIndicesRange.AddInt(72)
	randomIndicesRange.AddInt(73)
	randomIndicesRange.AddInt(74)
	randomIndicesRange.AddInt(76)

	ctx := context.Background()
	cases := []struct {
		name                  string
		startBlock            uint64
		endBlock              uint64
		stage                 int
		moduleName            string
		stateBundleSize       uint64
		manifestPath          string
		preCreatedFiles       []string
		preCreatedIndices     *preCreatedIndices
		expectRemainingFiles  []string
		mapOutputFileToCheck  string
		expectedSkippedBlocks map[uint64]struct{}
	}{
		// Complex substreams package : "./testdata/complex_substreams/complex-substreams-v0.1.0.spkg"
		// Output module : map_output_init_50
		//Stage 0: [["first_store_init_20"]]
		//Stage 1: [["second_store_init_30"]]
		//Stage 2: [["third_store_init_40"]]
		//Stage 3: [["map_output_init_50"]]
		{
			name:            "check full kv production in previous stages",
			startBlock:      50,
			endBlock:        60,
			stage:           3,
			moduleName:      "map_output_init_50",
			stateBundleSize: 10,
			manifestPath:    "./testdata/complex_substreams/complex-substreams-v0.1.0.spkg",
			preCreatedFiles: []string{
				firstStoreInit20 + "/states/0000000050-0000000020.kv.zst",
				secondStoreInit30 + "/states/0000000050-0000000030.kv.zst",
				thirdStoreInit40 + "/states/0000000050-0000000040.kv.zst",
			},

			expectRemainingFiles: []string{
				firstStoreInit20 + "/states/0000000060-0000000020.kv",
				secondStoreInit30 + "/states/0000000060-0000000030.kv",
				thirdStoreInit40 + "/states/0000000060-0000000040.kv",

				firstStoreInit20 + "/states/0000000050-0000000020.kv",
				firstStoreInit20 + "/outputs/0000000050-0000000060.output",
				secondStoreInit30 + "/states/0000000050-0000000030.kv",
				secondStoreInit30 + "/outputs/0000000050-0000000060.output",
				thirdStoreInit40 + "/states/0000000050-0000000040.kv",
				thirdStoreInit40 + "/outputs/0000000050-0000000060.output",
				mapInit50 + "/outputs/0000000050-0000000060.output",
			},
		},

		// Complex substreams package : "./testdata/complex_substreams/complex-substreams-v0.1.0.spkg"
		// Output module : second_map_output_init_50
		//Stage 0: [["first_store_init_20"]]
		//Stage 1: [["second_store_init_30"]]
		//Stage 2: [["third_store_init_40","fourth_store_init_52"]]
		//Stage 3: [["second_map_output_init_50"]]
		{
			name:            "stores with different initial blocks on the same stage",
			startBlock:      50,
			endBlock:        60,
			stage:           3,
			moduleName:      "second_map_output_init_50",
			stateBundleSize: 10,
			manifestPath:    "./testdata/complex_substreams/complex-substreams-v0.1.0.spkg",
			preCreatedFiles: []string{
				firstStoreInit20 + "/states/0000000050-0000000020.kv.zst",
				secondStoreInit30 + "/states/0000000050-0000000030.kv.zst",
				thirdStoreInit40 + "/states/0000000050-0000000040.kv.zst",
			},

			expectRemainingFiles: []string{
				firstStoreInit20 + "/states/0000000060-0000000020.kv",
				secondStoreInit30 + "/states/0000000060-0000000030.kv",
				thirdStoreInit40 + "/states/0000000060-0000000040.kv",

				firstStoreInit20 + "/states/0000000050-0000000020.kv",
				firstStoreInit20 + "/outputs/0000000050-0000000060.output",
				secondStoreInit30 + "/states/0000000050-0000000030.kv",
				secondStoreInit30 + "/outputs/0000000050-0000000060.output",
				thirdStoreInit40 + "/states/0000000050-0000000040.kv",
				thirdStoreInit40 + "/outputs/0000000050-0000000060.output",
				secondMapInit50 + "/outputs/0000000050-0000000060.output",

				fourthStoreInit52 + "/states/0000000060-0000000052.kv",
				fourthStoreInit52 + "/outputs/0000000052-0000000060.output",
			},
		},
		// This test is checking the index file loading when file already existing
		// Complex substreams package : "./testdata/complex_substreams/complex-substreams-v0.1.0.spkg"
		// Output module : map_using_index with block filter on even keys
		//Stage 0: [["index"],["map_using_index"]]
		{
			name:            "test index_init_60 with map_using_index_init_70 filtering through key 'even' with pre-existing random indices",
			startBlock:      70,
			endBlock:        80,
			stage:           0,
			moduleName:      "map_using_index_init_70",
			stateBundleSize: 10,
			manifestPath:    "./testdata/complex_substreams/complex-substreams-v0.1.0.spkg",

			preCreatedIndices: &preCreatedIndices{
				fileName: blockIndexInit60 + "/index/0000000070-0000000080.index",
				indices:  map[string]*roaring64.Bitmap{"even": randomIndicesRange},
			},

			expectRemainingFiles: []string{
				mapUsingIndexInit70 + "/outputs/0000000070-0000000080.output",
				blockIndexInit60 + "/index/0000000070-0000000080.index",
			},

			mapOutputFileToCheck:  mapUsingIndexInit70 + "/outputs/0000000070-0000000080.output",
			expectedSkippedBlocks: map[uint64]struct{}{75: {}, 77: {}, 78: {}, 79: {}, 80: {}},
		},

		// Complex substreams package : "./testdata/complex_substreams/complex-substreams-v0.1.0.spkg"
		// Output module : map_using_index with block filter on even keys
		//Stage 0: [["index"],["map_using_index"]]
		{
			name:            "test index_init_60 with map_using_index_init_70 filtering through key 'even'",
			startBlock:      70,
			endBlock:        80,
			stage:           0,
			moduleName:      "map_using_index_init_70",
			stateBundleSize: 10,
			manifestPath:    "./testdata/complex_substreams/complex-substreams-v0.1.0.spkg",

			expectRemainingFiles: []string{
				blockIndexInit60 + "/index/0000000070-0000000080.index",
				mapUsingIndexInit70 + "/outputs/0000000070-0000000080.output",
			},

			mapOutputFileToCheck:  mapUsingIndexInit70 + "/outputs/0000000070-0000000080.output",
			expectedSkippedBlocks: map[uint64]struct{}{71: {}, 73: {}, 75: {}, 77: {}, 79: {}},
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			testTempDir := t.TempDir()

			extendedTempDir := filepath.Join(testTempDir, "test.store", "tag")
			err := createFiles(extendedTempDir, test.preCreatedFiles)
			require.NoError(t, err)

			if test.preCreatedIndices != nil {
				err = createIndexFile(ctx, extendedTempDir, test.preCreatedIndices.fileName, test.preCreatedIndices.indices)
				require.NoError(t, err)
			}

			pkg := manifest.TestReadManifest(t, test.manifestPath)

			ctx = reqctx.WithRequest(ctx, &reqctx.RequestDetails{Modules: pkg.Modules, OutputModule: test.moduleName})

			ctx = reqctx.WithTier2RequestParameters(ctx, reqctx.Tier2RequestParameters{
				BlockType:            "sf.substreams.v1.test.Block",
				StateBundleSize:      test.stateBundleSize,
				StateStoreURL:        filepath.Join(testTempDir, "test.store"),
				StateStoreDefaultTag: "tag",
			})

			responseCollector := newResponseCollector()

			newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
				return &LinearBlockGenerator{
					startBlock:         startBlock,
					inclusiveStopBlock: inclusiveStopBlock,
				}
			}

			workRange := block.NewRange(test.startBlock, test.endBlock)

			request := work.NewRequest(ctx, reqctx.Details(ctx), test.stage, workRange)

			err = processInternalRequest(t, ctx, request, nil, newBlockGenerator, responseCollector, nil, testTempDir)
			require.NoError(t, err)

			withZST := func(s []string) []string {
				res := make([]string, len(s))
				for i, v := range s {
					res[i] = fmt.Sprintf("%s.zst", v)
				}
				return res
			}

			assertFiles(t, testTempDir, false, withZST(test.expectRemainingFiles)...)

			outputFileToCheck := test.mapOutputFileToCheck
			if outputFileToCheck != "" {
				err = checkBlockSkippedInOutputFile(ctx, extendedTempDir, outputFileToCheck, test.expectedSkippedBlocks)
			}
			require.NoError(t, err)
		})
	}
}

func createFiles(extendedTempDir string, files []string) error {
	for _, file := range files {
		_, err := createFile(extendedTempDir, file)
		if err != nil {
			return err
		}
	}
	return nil
}

func createFile(extendedTempDir string, file string) (*os.File, error) {
	desiredPath := filepath.Join(extendedTempDir, file)

	err := os.MkdirAll(filepath.Dir(desiredPath), os.ModePerm)
	if err != nil {
		return nil, err
	}

	createdFile, err := os.Create(desiredPath)
	if err != nil {
		return nil, err
	}

	return createdFile, nil
}

func checkBlockSkippedInOutputFile(ctx context.Context, extendedTempDir, checkedFile string, expectedSkippedBlock map[uint64]struct{}) error {
	s, err := dstore.NewStore(extendedTempDir, "zst", "zstd", false)
	if err != nil {
		return fmt.Errorf("initializing dstore for %q: %w", extendedTempDir, err)
	}

	fileReader, err := s.OpenObject(ctx, checkedFile)
	if err != nil {
		return fmt.Errorf("opening file %w", err)
	}

	ctn, err := io.ReadAll(fileReader)
	if err != nil {
		return fmt.Errorf("reading store file %w", err)
	}

	outputData := &pboutput.Map{}

	if err = outputData.UnmarshalFast(ctn); err != nil {
		return fmt.Errorf("unmarshalling file %s: %w", checkedFile, err)
	}

	for _, item := range outputData.Kv {
		_, found := expectedSkippedBlock[item.BlockNum]
		if found {
			return fmt.Errorf("block %d should have been skipped", item.BlockNum)
		}
	}

	return nil
}

func createIndexFile(ctx context.Context, extendedTempDir string, filename string, indices map[string]*roaring64.Bitmap) error {
	data, err := index.ConvertIndexesMapToBytes(indices)
	if err != nil {
		return fmt.Errorf("converting indices into bytes")
	}

	pbIndexesMap := pbindexes.Map{Indexes: data}
	cnt, err := proto.Marshal(&pbIndexesMap)
	if err != nil {
		return fmt.Errorf("marshalling Indices: %w", err)
	}

	store, err := dstore.NewStore(extendedTempDir, "zst", "zstd", false)
	if err != nil {
		return fmt.Errorf("initializing dstore for %q: %w", extendedTempDir, err)
	}

	reader := bytes.NewReader(cnt)
	err = store.WriteObject(ctx, filename, reader)
	if err != nil {
		return fmt.Errorf("writing file %s : %w", filename, err)
	}

	return nil
}
