package tools

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/streamingfast/cli"
	"github.com/streamingfast/derr"
	"github.com/streamingfast/substreams/manifest"
	store2 "github.com/streamingfast/substreams/storage/store"
	"go.uber.org/zap"

	"github.com/spf13/cobra"
	"github.com/streamingfast/dstore"
)

var moduleCmd = &cobra.Command{
	Use:   "module <module_name> [<manifest|spkg_path>] <substreams_state_store_url>",
	Short: "returns the state of the module on the state store",
	Long: cli.Dedent(`
		Returns the state of the module on the state store. The manifest is optional as it will try to find a file named 
		'substreams.yaml' in current working directory if nothing entered. You may enter a directory that contains a 'substreams.yaml' 
		file in place of '<manifest_file>'.
	`),
	Args: cobra.RangeArgs(2, 3),
	RunE: moduleRunE,
}

func init() {
	Cmd.AddCommand(moduleCmd)
}

func moduleRunE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	moduleName := args[0]
	manifestPathRaw := ""
	if len(args) == 3 {
		manifestPathRaw = args[1]
		args = args[1:]
	}

	stateStoreURL := args[1]
	manifestPath, err := ResolveManifestFile(manifestPathRaw)
	if err != nil {
		return fmt.Errorf("resolving manifest: %w", err)
	}

	zlog.Info("found state store",
		zap.String("module_name", moduleName),
		zap.String("manifest_path", manifestPath),
		zap.String("state_store_url", stateStoreURL),
	)

	stateStore, err := dstore.NewStore(stateStoreURL, "zst", "zstd", false)
	cli.NoError(err, "New state store")

	zlog.Info("Reading Substreams manifest")
	manifestReader := manifest.NewReader(manifestPath)
	pkg, err := manifestReader.Read()
	cli.NoError(err, "Read Substreams manifest")

	graph, err := manifest.NewModuleGraph(pkg.Modules.Modules)
	cli.NoError(err, "Create Substreams module graph")

	module, err := graph.Module(moduleName)
	cli.NoError(err, "unable to get module")

	kind := "STORE"
	if module.GetKindMap() != nil {
		kind = "MAP"
	}

	moduleHashes := manifest.NewModuleHashes()
	moduleHash := hex.EncodeToString(moduleHashes.HashModule(pkg.Modules, module, graph))

	outputModuleStore, err := stateStore.SubStore(fmt.Sprintf("%s/outputs", moduleHash))
	if err != nil {
		return fmt.Errorf("failed createing substore: %w", err)
	}

	outputFiles, err := walkFiles(
		ctx,
		outputModuleStore,
		func(filename string) string {
			return filename
		},
	)

	cli.NoError(err, "unable to output files")

	//outputFileToPrint := outputFiles
	//if len(outputFiles) > 6 {
	//	outputFileToPrint = []*fileInfo{}
	//	for _, file := range outputFiles[:5] {
	//		outputFileToPrint = append(outputFileToPrint, file)
	//	}
	//	for _, file := range outputFiles[len(outputFiles)-5:] {
	//		outputFileToPrint = append(outputFileToPrint, file)
	//	}
	//}

	var storeFullKVFiles []string
	var storepartialKVFiles []string

	if kind == "STORE" {
		store, err := store2.NewConfig(
			module.Name,
			module.InitialBlock,
			moduleHash,
			module.GetKindStore().UpdatePolicy,
			module.GetKindStore().ValueType,
			stateStore,
		)
		cli.NoError(err, "unable to create store config")

		out, err := store.ListSnapshotFiles(ctx)
		cli.NoError(err, "list snapshots")
		for _, o := range out {
			if o.Partial {
				storepartialKVFiles = append(
					storepartialKVFiles,
					o.Filename,
				)
				continue
			}
			storeFullKVFiles = append(
				storeFullKVFiles,
				o.Filename,
			)
		}
	}

	fmt.Println("")
	fmt.Printf("Module: %s [%s]\n", module.Name, kind)
	fmt.Printf("Hash: %s\n", moduleHash)
	fmt.Printf("Start RecordBlock: %d\n", module.InitialBlock)
	fmt.Printf("Output Files: %d Ouput files found\n", len(outputFiles))
	displayList(outputFiles)

	if kind == "STORE" {
		fmt.Println("")
		fmt.Printf("Store Files: \n")
		fmt.Printf("Full KV Files Count: %d\n", len(storeFullKVFiles))
		displayList(storeFullKVFiles)
		fmt.Println("")
		fmt.Printf("Partial KV Files Count: %d\n", len(storepartialKVFiles))
		displayList(storepartialKVFiles)
	}
	return nil
}

func walkFiles(ctx context.Context, store dstore.Store, processor func(filename string) string) (files []string, err error) {
	err = derr.RetryContext(ctx, 3, func(ctx context.Context) error {
		if err := store.Walk(ctx, "", func(filename string) (err error) {
			files = append(files, processor(filename))
			return nil
		}); err != nil {
			return fmt.Errorf("walking snapshots: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

const MAX_LIST_DISPLAY_COUNT = 6

func displayList(list []string) {
	if len(list) < MAX_LIST_DISPLAY_COUNT {
		for _, l := range list {
			fmt.Printf("    - %s\n", l)
		}
		return
	}

	for _, l := range list[:(MAX_LIST_DISPLAY_COUNT - 1)] {
		fmt.Printf("    - %s\n", l)
	}
	fmt.Printf("    -......\n")
	for _, l := range list[len(list)-MAX_LIST_DISPLAY_COUNT-1:] {
		fmt.Printf("    - %s\n", l)
	}
}
