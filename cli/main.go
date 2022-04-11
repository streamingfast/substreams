package cli

import "fmt"

func Main() {
	setup()

	err := rootCmd.Execute()
	if err != nil {
		fmt.Println("Error:", err)
		//zlog.Error("running cmd", zap.Error(err))
	}
}
