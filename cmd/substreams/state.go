package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/spf13/cobra"
	"github.com/streamingfast/substreams/tools"
)

func init() {
	stateCmd.Flags().String("state-store-url", "./localdata", "URL of state store")

	tools.Cmd.AddCommand(stateCmd)
}

// localCmd represents the base command when called without any subcommands
var stateCmd = &cobra.Command{
	Use:          "state <state_file_name>",
	RunE:         runState,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
}

func runState(cmd *cobra.Command, args []string) error {

	kvFileNamePath := args[0]
	file, err := os.Open(kvFileNamePath)
	if err != nil {
		log.Panicf("failed reading file: %s", err)
	}
	defer file.Close()
	data, err := ioutil.ReadAll(file)
	if err != nil {
		panic(err)
	}

	fmt.Printf("\nLength: %d bytes", len(data))
	fmt.Printf("\nData: %s", data)

	kv := map[string]string{}
	if err = json.Unmarshal(data, &kv); err != nil {
		panic(err)
	}
	fmt.Println("entry count", len(kv))
	return nil
}
