package cli

func Main(version string) {
	rootCmd.Version = version

	setup()

	err := rootCmd.Execute()
	if err != nil {
		//fmt.Println("Error:", err)
		//zlog.Error("running cmd", zap.Error(err))
	}
}
