package integration

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/streamingfast/substreams/manifest"

	"github.com/streamingfast/substreams/block"
	"github.com/stretchr/testify/require"

	"github.com/streamingfast/substreams/orchestrator/work"
	"github.com/streamingfast/substreams/reqctx"
)

func TestTier2Call(t *testing.T) {
	manifest.UseSimpleHash = true
	testMap := hex.EncodeToString([]byte("index"))
	mapInit50 := hex.EncodeToString([]byte("map_output_init_50"))
	secondMapInit50 := hex.EncodeToString([]byte("second_map_output_init_50"))

	firstStoreInit20 := hex.EncodeToString([]byte("first_store_init_20"))
	secondStoreInit30 := hex.EncodeToString([]byte("second_store_init_30"))
	thirdStoreInit40 := hex.EncodeToString([]byte("third_store_init_40"))
	//fourthStoreInit50 := hex.EncodeToString([]byte("fourth_store_init_50"))

	ctx := context.Background()
	cases := []struct {
		name                 string
		startBlock           uint64
		endBlock             uint64
		stage                int
		moduleName           string
		stateBundleSize      uint64
		manifestPath         string
		preCreatedFiles      []string
		expectRemainingFiles []string
	}{
		{
			name:            "test1",
			startBlock:      10,
			endBlock:        20,
			stage:           0,
			moduleName:      "index",
			stateBundleSize: 10,
			manifestPath:    "./testdata/complex_substreams/complex-substreams-v0.1.0.spkg",
			expectRemainingFiles: []string{
				testMap + "/outputs/0000000010-0000000020.output",
			},
		},

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
		//Stage 2: [["third_store_init_40","fourth_store_init_50"]]
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
			},
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			testTempDir := t.TempDir()

			extendedTempDir := filepath.Join(testTempDir, "test.store", "tag")
			err := createFiles(extendedTempDir, test.preCreatedFiles)
			require.NoError(t, err)

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
			require.NoError(t, err)
		})
	}
}

func createFiles(extendedTempDir string, files []string) error {
	for _, file := range files {
		err := createFile(extendedTempDir, file)
		if err != nil {
			return err
		}
	}
	return nil
}

func createFile(extendedTempDir string, file string) error {
	desiredPath := filepath.Join(extendedTempDir, file)

	err := os.MkdirAll(filepath.Dir(desiredPath), os.ModePerm)
	if err != nil {
		return err
	}

	_, err = os.Create(desiredPath)
	if err != nil {
		return err
	}

	return nil
}
