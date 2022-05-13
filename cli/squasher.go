package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/streamingfast/dstore"
)

var squasherCmd = &cobra.Command{
	Use:  "squasher <base_store_dsn> <modules_list>",
	Args: cobra.ExactArgs(2),
	RunE: runSquashE,
}

func init() {
	rootCmd.AddCommand(squasherCmd)
}

type SquasherConfig struct {
	Modules []string `json:"modules"`
}

func NewSquasherConfig(modules ...string) *SquasherConfig {
	return &SquasherConfig{Modules: modules}
}

type Squasher struct {
	config *SquasherConfig
}

func NewSquasher(config *SquasherConfig) *Squasher {
	return &Squasher{config: config}
}

func (s *Squasher) run(ctx context.Context, baseStore dstore.Store) error {
	panic("Squasher is dead because of builder refactoring")
	//ctx, cancel := context.WithCancel(ctx)
	//defer cancel()
	//
	//eg := llerrgroup.New(len(s.config.Modules))
	//
	//for _, store := range s.config.Modules {
	//	if eg.Stop() {
	//		continue
	//	}
	//
	//	storeName := store
	//	eg.Go(func() (perr error) {
	//		defer func() {
	//			if perr != nil {
	//				cancel()
	//			}
	//		}()
	//
	//		//get metadata file
	//		metadataFileName := state.StateInfoFileName()
	//		exists, basePath, err := findUniqueFile(ctx, baseStore, metadataFileName)
	//		if err != nil {
	//			perr = fmt.Errorf("finding file %s: %w", metadataFileName, err)
	//			return
	//		}
	//		fmt.Println(basePath)
	//
	//		if !exists {
	//			perr = fmt.Errorf("metadata file %s does not exist", metadataFileName)
	//			return
	//		}
	//
	//	Loop:
	//		for {
	//			select {
	//			case <-ctx.Done():
	//				break Loop
	//			default:
	//
	//			}
	//
	//			metadataFileBytes, err := readObject(ctx, baseStore, basePath, metadataFileName)
	//			if err != nil {
	//				perr = fmt.Errorf("reading object %s: %w", metadataFileName, err)
	//				return
	//			}
	//
	//			var metadata *state.Info
	//			err = json.Unmarshal(metadataFileBytes, &metadata)
	//			if err != nil {
	//				perr = fmt.Errorf("unmarshalling metadata %s: %w", metadataFileName, err)
	//				return
	//			}
	//
	//			fileinfo, ok := state.ParseFileName(metadata.LastKVFile)
	//			if !ok {
	//				perr = fmt.Errorf("could not parse filename %s", metadata.LastKVFile)
	//				return
	//			}
	//			kvFileEndBlock := fileinfo.EndBlock
	//
	//			partialFileStartBlock := kvFileEndBlock
	//			partialFileEndBlock := kvFileEndBlock + uint64(metadata.RangeIntervalSize)
	//
	//			//wait for next partial file to appear
	//			partialSubstore, err := baseStore.SubStore(basePath)
	//			if err != nil {
	//				perr = fmt.Errorf("getting substore: %w", err)
	//				return
	//			}
	//
	//			<-state.WaitPartial(ctx, storeName, partialSubstore, partialFileStartBlock, partialFileEndBlock)
	//			partialFileName := state.PartialFileName(partialFileStartBlock, partialFileEndBlock)
	//
	//			//open the files
	//			partial, err := state.NewBuilderFromFile(ctx, strings.Join([]string{basePath, partialFileName}, string(filepath.Separator)), baseStore)
	//			if err != nil {
	//				perr = fmt.Errorf("creating partial state: %w", err)
	//				return
	//			}
	//
	//			kv, err := state.NewBuilderFromFile(ctx, strings.Join([]string{basePath, metadata.LastKVFile}, string(filepath.Separator)), baseStore)
	//			if err != nil {
	//				perr = fmt.Errorf("creating kv state: %w", err)
	//				return
	//			}
	//
	//			//squash them
	//			err = partial.Merge(kv)
	//			if err != nil {
	//				perr = fmt.Errorf("merging: %w", err)
	//				return
	//			}
	//
	//			//save the result
	//			mergedFilename, err := partial.WriteState(ctx, partialFileEndBlock, false)
	//			if err != nil {
	//				perr = fmt.Errorf("writing new kv state: %w", err)
	//				return
	//			}
	//
	//			//delete the partial
	//			err = deleteObject(ctx, baseStore, basePath, partialFileName)
	//			if err != nil {
	//				perr = fmt.Errorf("deleting partial file %s: %w", partialFileName, err)
	//				return
	//			}
	//
	//			//update and save metadata
	//			metadata.LastKVFile = mergedFilename
	//
	//			newMetadataBytes, err := json.Marshal(metadata)
	//			if err != nil {
	//				perr = fmt.Errorf("json marshaling metadata: %w", err)
	//				return
	//			}
	//
	//			err = writeObject(ctx, baseStore, basePath, metadataFileName, newMetadataBytes)
	//			if err != nil {
	//				perr = fmt.Errorf("writing metadata file %s: %w", metadataFileName, err)
	//				return
	//			}
	//
	//			time.Sleep(10 * time.Second)
	//		}
	//
	//		return nil
	//	})
	//}
	//
	//err := eg.Wait()
	//if err != nil {
	//	return fmt.Errorf("running scheduler: %w", err)
	//}
	//
	//return nil
}

func findUniqueFile(ctx context.Context, baseStore dstore.Store, filename string) (exists bool, basePath string, err error) {
	exists = false
	var files []string

	_ = baseStore.Walk(ctx, "", "", func(storeFile string) (err error) {
		if !strings.HasSuffix(storeFile, fmt.Sprintf("%s%s", string(filepath.Separator), filename)) {
			return
		}
		files = append(files, storeFile)
		exists = true
		return
	})

	if len(files) != 1 {
		return false, "", fmt.Errorf("invalid result. length should be 1, got %d", len(files))
	}

	file := files[0]
	return true, getBasePath(filename, file), nil
}

func getBasePath(filename, objectPath string) string {
	path := strings.TrimSuffix(objectPath, filename)
	return strings.Trim(path, string(filepath.Separator))
}

func deleteObject(ctx context.Context, store dstore.Store, basePath, objectName string) error {
	return store.DeleteObject(ctx, strings.Join([]string{basePath, objectName}, string(filepath.Separator)))
}

func readObject(ctx context.Context, store dstore.Store, basePath, objectName string) ([]byte, error) {
	rc, err := store.OpenObject(ctx, strings.Join([]string{basePath, objectName}, string(filepath.Separator)))
	if err != nil {
		return nil, fmt.Errorf("opening object")
	}
	defer rc.Close()

	return io.ReadAll(rc)
}

func writeObject(ctx context.Context, store dstore.Store, basePath, objectName string, objectData []byte) error {
	br := bytes.NewReader(objectData)

	err := store.WriteObject(ctx, strings.Join([]string{basePath, objectName}, string(filepath.Separator)), br)
	if err != nil {
		return fmt.Errorf("writing object")
	}

	return nil
}

func runSquashE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	modulesList := strings.Split(args[1], ",")
	if len(modulesList) == 0 {
		return fmt.Errorf("modules list is empty")
	}

	config := NewSquasherConfig(modulesList...)
	squasher := NewSquasher(config)

	store, err := dstore.NewStore(args[0], "", "", false)
	if err != nil {
		return fmt.Errorf("creating store: %w", err)
	}

	err = squasher.run(ctx, store)
	if err != nil {
		return err
	}
	return nil
}
