package cli

import (
	"go.uber.org/zap"
)

func Main() {
	setup()

	err := rootCmd.Execute()
	if err != nil {
		zlog.Error("running cmd", zap.Error(err))
	}
}
