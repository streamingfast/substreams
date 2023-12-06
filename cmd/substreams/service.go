package main

import "github.com/spf13/cobra"

func init() {
	alphaCmd.AddCommand(serviceCmd)
	serviceCmd.PersistentFlags().StringP("endpoint", "e", "http://localhost:8000", "specify the endpoint to connect to.")
	serviceCmd.PersistentFlags().Bool("strict", false, "Require deploymentID parameter to be set and complete")
	serviceCmd.PersistentFlags().StringSliceP("header", "H", nil, "Additional headers to be sent in the substreams request (ex: 'X-Substreams-my-var: my-value')")
	serviceCmd.PersistentFlags().String("substreams-api-token-envvar", "SUBSTREAMS_API_TOKEN", "Name of variable containing Substreams Authentication token, will be passed as 'authorization: bearer {}' flag")
}

var serviceCmd = &cobra.Command{
	Use: "service",
}
