package cli

import (
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func autoBind(root *cobra.Command, prefix string) {
	recurseCommands(root, "SUBSTREAMS", nil) // []string{strings.ToLower(prefix)}) how does it wweeeerrkk?
}

func recurseCommands(root *cobra.Command, prefix string, segments []string) {
	var segmentPrefix string
	if len(segments) > 0 {
		segmentPrefix = strings.ToUpper(strings.Join(segments, "_")) + "_"
	}

	root.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		newName := strings.Replace(strings.ToUpper(f.Name), "-", "_", -1)
		varName := prefix + "_" + segmentPrefix + "GLOBAL_" + newName
		//fmt.Println("PERSISTENT FLAG:", varName)
		if val := os.Getenv(varName); val != "" {
			f.Usage += " [LOADED FROM ENV]" // Until we have a better template for our usage.
			f.DefValue = val
		}
	})

	root.Flags().VisitAll(func(f *pflag.Flag) {
		newName := strings.Replace(strings.ToUpper(f.Name), "-", "_", -1)
		varName := prefix + "_" + segmentPrefix + "CMD_" + newName
		//fmt.Println("FLAG:", varName)
		if val := os.Getenv(varName); val != "" {
			f.Usage += " [LOADED FROM ENV]"
			f.DefValue = val
		}
	})

	for _, cmd := range root.Commands() {
		recurseCommands(cmd, prefix, append(segments, cmd.Name()))
	}
}
