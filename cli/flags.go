package cli

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"strings"
)

func autoBind(root *cobra.Command, prefix string) {
	viper.SetEnvPrefix(strings.ToUpper(prefix))
	viper.AutomaticEnv()
	replacer := strings.NewReplacer("-", "_")
	viper.SetEnvKeyReplacer(replacer)

	recurseCommands(root, nil)
}

func recurseCommands(root *cobra.Command, segments []string) {
	var segmentPrefix string
	if len(segments) > 0 {
		segmentPrefix = strings.Join(segments, "-") + "-"
	}

	zlog.Debug("re-binding flags", zap.String("cmd", root.Name()), zap.String("prefix", segmentPrefix))
	defer func() {
		zlog.Debug("reboung flags terminated", zap.String("cmd", root.Name()))
	}()

	root.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		newVar := segmentPrefix + "global-" + f.Name
		viper.BindPFlag(newVar, f)
		zlog.Debug("binding persistent flag", zap.String("actual", f.Name), zap.String("rebind", newVar))
	})

	root.Flags().VisitAll(func(f *pflag.Flag) {
		newVar := segmentPrefix + f.Name
		viper.BindPFlag(newVar, f)
		zlog.Debug("binding flag", zap.String("actual", f.Name), zap.String("rebind", newVar))
	})

	for _, cmd := range root.Commands() {
		recurseCommands(cmd, append(segments, cmd.Name()))
	}
}
