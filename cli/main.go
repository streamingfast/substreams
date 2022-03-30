package cli

import (
	"go.uber.org/zap"
)

func Main() {
	setup()

	autoBind(rootCmd, "SUBSTREAMS")

	err := rootCmd.Execute()
	if err != nil {
		zlog.Error("running cmd", zap.Error(err))
	}
}
