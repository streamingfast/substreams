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

	pbsubstreamstest "github.com/streamingfast/substreams/pb/sf/substreams/v1/test"
	pbindexes "github.com/streamingfast/substreams/storage/index/pb"
	"google.golang.org/protobuf/proto"

	"github.com/streamingfast/substreams/storage/index"

	"github.com/RoaringBitmap/roaring/roaring64"

	"github.com/streamingfast/dstore"

	pboutput "github.com/streamingfast/substreams/storage/execout/pb"

	"github.com/streamingfast/substreams/manifest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/streamingfast/substreams/orchestrator/work"
	"github.com/streamingfast/substreams/reqctx"
)

type preCreatedIndices struct {
	fileName string
	indices  map[string]*roaring64.Bitmap
}

func TestTier2Call(t *testing.T) {
	manifest.TestUseSimpleHash = true
	mapInit50 := hex.EncodeToString([]byte("map_output_init_50"))
	secondMapInit50 := hex.EncodeToString([]byte("second_map_output_init_50"))

	firstStoreInit20 := hex.EncodeToString([]byte("first_store_init_20"))
	secondStoreInit30 := hex.EncodeToString([]byte("second_store_init_30"))
	thirdStoreInit40 := hex.EncodeToString([]byte("third_store_init_40"))
	fourthStoreInit52 := hex.EncodeToString([]byte("fourth_store_init_52"))
	blockIndexInit60 := hex.EncodeToString([]byte("index_init_60"))
	mapUsingIndexInit70 := hex.EncodeToString([]byte("map_using_index_init_70"))
	mapHybridInputClock70 := hex.EncodeToString([]byte("map_hybrid_input_clock_70"))
	mapHybridInputBlock70 := hex.EncodeToString([]byte("map_hybrid_input_block_70"))
	setSumStoreInit0 := hex.EncodeToString([]byte("set_sum_store_init_0"))

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
		firstStreamableBlock  uint64
		stage                 int
		moduleName            string
		stateBundleSize       uint64
		manifestPath          string
		preCreatedFiles       []string
		preCreatedIndices     *preCreatedIndices
		expectRemainingFiles  []string
		mapOutputFileToCheck  string
		expectedSkippedBlocks map[uint64]struct{}

		mapOutputFilesToDeepInspectForKeys map[string]map[uint64]any
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
			mapOutputFilesToDeepInspectForKeys: map[string]map[uint64]any{
				mapInit50 + "/outputs/0000000050-0000000060.output": {
					50: &pbsubstreamstest.MapResult{BlockNumber: 1, BlockHash: "block-50"}, // blockNumber is just used as a counter in this mapper
					51: &pbsubstreamstest.MapResult{BlockNumber: 2, BlockHash: "block-51"},
					52: &pbsubstreamstest.MapResult{BlockNumber: 3, BlockHash: "block-52"},
					53: &pbsubstreamstest.MapResult{BlockNumber: 4, BlockHash: "block-53"},
					54: &pbsubstreamstest.MapResult{BlockNumber: 5, BlockHash: "block-54"},
					55: &pbsubstreamstest.MapResult{BlockNumber: 6, BlockHash: "block-55"},
					56: &pbsubstreamstest.MapResult{BlockNumber: 7, BlockHash: "block-56"},
					57: &pbsubstreamstest.MapResult{BlockNumber: 8, BlockHash: "block-57"},
					58: &pbsubstreamstest.MapResult{BlockNumber: 9, BlockHash: "block-58"},
					59: &pbsubstreamstest.MapResult{BlockNumber: 10, BlockHash: "block-59"},
				},
			},
		},

		// Simple substreams package with initialBlock==0 : "./testdata/simple_substreams_init0/substreams-test-init0-v0.1.0.spkg"
		// Output module : test_map
		//Stage 0: [["test_map"]]
		{
			name:                 "first streamble block",
			startBlock:           10,
			firstStreamableBlock: 18,
			stage:                0,
			moduleName:           "test_map",
			stateBundleSize:      10,
			manifestPath:         "./testdata/simple_substreams_init0/substreams-test-init0-v0.1.0.spkg",
			preCreatedFiles:      nil,

			expectRemainingFiles: []string{
				"746573745f6d6170/outputs/0000000018-0000000020.output",
			},
		},

		// Simple substreams package with initialBlock==0 : "./testdata/simple_substreams_init0/substreams-test-init0-v0.1.0.spkg"
		// Output module : test_map
		//Stage 0: [["test_store_add_i64"]]
		//Stage 1: [["assert_test_store_add_i64"]] (not run)
		{
			name:                 "first streamble block with store",
			startBlock:           10,
			firstStreamableBlock: 18,
			stage:                0,
			moduleName:           "assert_test_store_add_i64",
			stateBundleSize:      10,
			manifestPath:         "./testdata/simple_substreams_init0/substreams-test-init0-v0.1.0.spkg",
			preCreatedFiles:      nil,

			expectRemainingFiles: []string{
				"73657475705f746573745f73746f72655f6164645f693634/outputs/0000000018-0000000020.output",
				"73657475705f746573745f73746f72655f6164645f693634/states/0000000020-0000000018.partial",
			},
		},

		// Simple substreams package with initialBlock==0 : "./testdata/simple_substreams_init0/substreams-test-init0-v0.1.0.spkg"
		// Output module : test_map
		//Stage 0: [["test_store_add_i64"]]
		//Stage 1: [["assert_test_store_add_i64"]]
		{
			name:                 "first streamble block with store all stages together",
			startBlock:           10,
			firstStreamableBlock: 18,
			stage:                1,
			moduleName:           "assert_test_store_add_i64",
			stateBundleSize:      10,
			manifestPath:         "./testdata/simple_substreams_init0/substreams-test-init0-v0.1.0.spkg",
			preCreatedFiles:      nil,

			expectRemainingFiles: []string{
				"73657475705f746573745f73746f72655f6164645f693634/outputs/0000000018-0000000020.output",
				"73657475705f746573745f73746f72655f6164645f693634/states/0000000020-0000000018.kv", // kv store done directly
				"6173736572745f746573745f73746f72655f6164645f693634/outputs/0000000018-0000000020.output",
			},
		},

		// Simple substreams package with initialBlock==0 : "./testdata/simple_substreams_init0/substreams-test-init0-v0.1.0.spkg"
		// Output module : test_map
		//Stage 0: [["test_store_add_i64"]]
		//Stage 1: [["assert_test_store_add_i64"]] (not run)
		{
			name:                 "first streamble block with store second segment",
			startBlock:           20,
			firstStreamableBlock: 18,
			stage:                0,
			moduleName:           "assert_test_store_add_i64",
			stateBundleSize:      10,
			manifestPath:         "./testdata/simple_substreams_init0/substreams-test-init0-v0.1.0.spkg",
			preCreatedFiles: []string{
				"73657475705f746573745f73746f72655f6164645f693634/outputs/0000000018-0000000020.output.zst",
				"73657475705f746573745f73746f72655f6164645f693634/states/0000000020-0000000018.partial.zst",
			},

			expectRemainingFiles: []string{
				"73657475705f746573745f73746f72655f6164645f693634/outputs/0000000018-0000000020.output",
				"73657475705f746573745f73746f72655f6164645f693634/states/0000000020-0000000018.partial",

				"73657475705f746573745f73746f72655f6164645f693634/outputs/0000000020-0000000030.output",
				"73657475705f746573745f73746f72655f6164645f693634/states/0000000030-0000000020.partial",
			},
		},

		// Simple substreams package with initialBlock==0 : "./testdata/simple_substreams_init0/substreams-test-init0-v0.1.0.spkg"
		// Output module : test_map
		//Stage 0: [["test_store_add_i64"]]
		//Stage 1: [["assert_test_store_add_i64"]]
		{
			name:                 "first streamble block with store second stage",
			startBlock:           20,
			firstStreamableBlock: 18,
			stage:                1,
			moduleName:           "assert_test_store_add_i64",
			stateBundleSize:      10,
			manifestPath:         "./testdata/simple_substreams_init0/substreams-test-init0-v0.1.0.spkg",
			preCreatedFiles: []string{
				"73657475705f746573745f73746f72655f6164645f693634/outputs/0000000018-0000000020.output.zst",
				"73657475705f746573745f73746f72655f6164645f693634/states/0000000020-0000000018.kv.zst",
			},

			expectRemainingFiles: []string{
				"73657475705f746573745f73746f72655f6164645f693634/outputs/0000000018-0000000020.output",
				"73657475705f746573745f73746f72655f6164645f693634/states/0000000020-0000000018.kv",

				"73657475705f746573745f73746f72655f6164645f693634/outputs/0000000020-0000000030.output",
				"73657475705f746573745f73746f72655f6164645f693634/states/0000000030-0000000018.kv",

				"6173736572745f746573745f73746f72655f6164645f693634/outputs/0000000020-0000000030.output",
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
			expectedSkippedBlocks: map[uint64]struct{}{75: {}, 77: {}, 78: {}, 79: {}, 80: {}}, // faked with the randomIndicesRange above
		},
		// This test checks that a module receiving data from both a filtered map and a Clock
		// does not trigger on every block, even when the index is being created in the same run
		{
			name:            "hybrid input with clock and filtered map",
			startBlock:      70,
			stage:           0,
			moduleName:      "map_hybrid_input_clock_70",
			stateBundleSize: 10,
			manifestPath:    "./testdata/complex_substreams/complex-substreams-v0.1.0.spkg",

			expectRemainingFiles: []string{
				mapUsingIndexInit70 + "/outputs/0000000070-0000000080.output",
				mapHybridInputClock70 + "/outputs/0000000070-0000000080.output",
				blockIndexInit60 + "/index/0000000070-0000000080.index",
			},

			mapOutputFilesToDeepInspectForKeys: map[string]map[uint64]any{
				mapHybridInputClock70 + "/outputs/0000000070-0000000080.output": {
					70: &pbsubstreamstest.Boolean{Result: true},
					72: &pbsubstreamstest.Boolean{Result: true},
					74: &pbsubstreamstest.Boolean{Result: true},
					76: &pbsubstreamstest.Boolean{Result: true},
					78: &pbsubstreamstest.Boolean{Result: true},
				},
			},
		},

		// This test checks that a module receiving data from both a filtered map and another source
		// does not keep ghost values on every block, even when the index is being created in the same run
		{
			name:            "hybrid input with block and filtered map",
			startBlock:      70,
			stage:           0,
			moduleName:      "map_hybrid_input_block_70",
			stateBundleSize: 10,
			manifestPath:    "./testdata/complex_substreams/complex-substreams-v0.1.0.spkg",

			expectRemainingFiles: []string{
				mapUsingIndexInit70 + "/outputs/0000000070-0000000080.output",
				mapHybridInputBlock70 + "/outputs/0000000070-0000000080.output",
				blockIndexInit60 + "/index/0000000070-0000000080.index",
			},

			mapOutputFilesToDeepInspectForKeys: map[string]map[uint64]any{
				mapHybridInputBlock70 + "/outputs/0000000070-0000000080.output": {
					70: &pbsubstreamstest.Boolean{Result: true},
					71: &pbsubstreamstest.Boolean{Result: false},
					72: &pbsubstreamstest.Boolean{Result: true},
					73: &pbsubstreamstest.Boolean{Result: false},
					74: &pbsubstreamstest.Boolean{Result: true},
					75: &pbsubstreamstest.Boolean{Result: false},
					76: &pbsubstreamstest.Boolean{Result: true},
					77: &pbsubstreamstest.Boolean{Result: false},
					78: &pbsubstreamstest.Boolean{Result: true},
					79: &pbsubstreamstest.Boolean{Result: false},
				},
			},
		},
		{
			name:            "multi_store_different_40_start_0",
			startBlock:      0,
			stage:           0,
			moduleName:      "multi_store_different_40",
			stateBundleSize: 10,
			manifestPath:    "./testdata/complex_substreams/complex-substreams-v0.1.0.spkg",

			expectRemainingFiles: []string{
				//firstStoreInit20
				setSumStoreInit0 + "/states/0000000010-0000000000.partial",
				setSumStoreInit0 + "/outputs/0000000000-0000000010.output",
			},
		},

		{
			name:            "multi_store_different_40_start_10",
			startBlock:      10,
			stage:           0,
			moduleName:      "multi_store_different_40",
			stateBundleSize: 10,
			manifestPath:    "./testdata/complex_substreams/complex-substreams-v0.1.0.spkg",

			expectRemainingFiles: []string{
				//firstStoreInit20
				setSumStoreInit0 + "/outputs/0000000010-0000000020.output",
				setSumStoreInit0 + "/states/0000000020-0000000010.partial",
			},
		},

		// Complex substreams package : "./testdata/complex_substreams/complex-substreams-v0.1.0.spkg"
		// Output module : map_using_index with block filter on even keys
		//Stage 0: [["index"],["map_using_index"]]
		{
			name:            "test index_init_60 with map_using_index_init_70 filtering through key 'even'",
			startBlock:      70,
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
				FirstStreamableBlock: test.firstStreamableBlock,
				StateBundleSize:      test.stateBundleSize,
				StateStoreURL:        filepath.Join(testTempDir, "test.store"),
				MeteringConfig:       "some_metering_config",
				MergedBlockStoreURL:  "some_merged_block_store_url",
				StateStoreDefaultTag: "tag",
			})

			responseCollector := newResponseCollector(ctx)

			newBlockGenerator := func(startBlock uint64, inclusiveStopBlock uint64) TestBlockGenerator {
				return &LinearBlockGenerator{
					startBlock:         startBlock,
					inclusiveStopBlock: inclusiveStopBlock,
				}
			}

			request := work.NewRequest(ctx, reqctx.Details(ctx), test.stage, test.startBlock)
			require.NoError(t, request.Validate())

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

			for outputFile, expectedKeys := range test.mapOutputFilesToDeepInspectForKeys {
				assert.NoError(t, deepInspectOutputFile(ctx, extendedTempDir, outputFile, expectedKeys))
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

func deepInspectOutputFile(ctx context.Context, extendedTempDir, outputFile string, expectedResults map[uint64]any) error {
	outputData, err := readOutputFile(ctx, extendedTempDir, outputFile)
	if err != nil {
		return err
	}

	seenBlocks := make(map[uint64]struct{})
	for _, item := range outputData.Kv {
		seenBlocks[item.BlockNum] = struct{}{}

		expected, found := expectedResults[item.BlockNum]
		if !found {
			return fmt.Errorf("expected block %d to be skipped", item.BlockNum)
		}
		switch v := expected.(type) {
		case *pbsubstreamstest.MapResult:
			res := pbsubstreamstest.MapResult{}
			if err := proto.Unmarshal(item.Payload, &res); err != nil {
				return fmt.Errorf("unmarshaling payload at block %d: %w", item.BlockNum, err)
			}

			if expectedResults[item.BlockNum] == nil {
				return fmt.Errorf("unexpected block number %d", item.BlockNum)
			}
			if !proto.Equal(&res, v) {
				return fmt.Errorf("results do not match for block %d: expected %+v, got %+v", item.BlockNum, expectedResults[item.BlockNum], res)
			}
		case *pbsubstreamstest.Boolean:
			res := pbsubstreamstest.Boolean{}
			if err := proto.Unmarshal(item.Payload, &res); err != nil {
				return fmt.Errorf("unmarshaling payload at block %d: %w", item.BlockNum, err)
			}

			if expectedResults[item.BlockNum] == nil {
				return fmt.Errorf("unexpected block number %d", item.BlockNum)
			}
			if !proto.Equal(&res, v) {
				return fmt.Errorf("results do not match for block %d: expected %+v, got %+v", item.BlockNum, expectedResults[item.BlockNum], res)
			}
		default:
			return fmt.Errorf("unexpected type %T", v)
		}
	}

	for k := range expectedResults {
		if _, found := seenBlocks[k]; !found {
			return fmt.Errorf("block %d not found in output file", k)
		}
	}
	return nil

}

func readOutputFile(ctx context.Context, extendedTempDir, outputFile string) (*pboutput.Map, error) {
	s, err := dstore.NewStore(extendedTempDir, "zst", "zstd", false)
	if err != nil {
		return nil, fmt.Errorf("initializing dstore for %q: %w", extendedTempDir, err)
	}

	fileReader, err := s.OpenObject(ctx, outputFile)
	if err != nil {
		return nil, fmt.Errorf("opening file %w", err)
	}

	ctn, err := io.ReadAll(fileReader)
	if err != nil {
		return nil, fmt.Errorf("reading store file %w", err)
	}

	outputData := &pboutput.Map{}

	if err = outputData.UnmarshalFast(ctn); err != nil {
		return nil, fmt.Errorf("unmarshalling file %s: %w", outputFile, err)
	}
	return outputData, nil
}

func checkBlockSkippedInOutputFile(ctx context.Context, extendedTempDir, checkedFile string, expectedSkippedBlock map[uint64]struct{}) error {
	outputData, err := readOutputFile(ctx, extendedTempDir, checkedFile)
	if err != nil {
		return err
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
