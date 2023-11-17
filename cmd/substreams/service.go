package main

import "github.com/spf13/cobra"

func init() {
	alphaCmd.AddCommand(serviceCmd)
	serviceCmd.PersistentFlags().StringP("endpoint", "e", "http://localhost:8000", "specify the endpoint to connect to.")
	serviceCmd.PersistentFlags().Bool("strict", false, "Require deploymentID parameter to be set and complete")
	serviceCmd.PersistentFlags().StringSliceP("header", "H", nil, "Additional headers to be sent in the substreams request (ex: 'X-Substreams-Api-Key: api_1234567')")
}

var serviceCmd = &cobra.Command{
	Use: "service",
}
