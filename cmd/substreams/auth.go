package main

import (
	"fmt"
	"os"

	"github.com/fatih/color"

	"github.com/spf13/cobra"
)

var authCmd = &cobra.Command{
	Use:          "auth",
	Short:        "Login command for Substreams development",
	RunE:         runAuthE,
	SilenceUsage: true,
}

func init() {
	rootCmd.AddCommand(authCmd)
}

func runAuthE(cmd *cobra.Command, args []string) error {
	localDevelopment := os.Getenv("LOCAL_DEVELOPMENT")

	fmt.Println("Open this link to authenticate on The Graph Market:")
	if localDevelopment == "true" {
		color.Blue("http://localhost:3000/dev-onboarding")
	} else {
		color.Blue("https://thegraph.market/dev-onboarding")
	}

	fmt.Println()
	fmt.Print("Then paste the token here: ")

	var token string
	_, err := fmt.Scanln(&token)
	if err != nil {
		return fmt.Errorf("error reading token: %w", err)
	}

	if token == "" {
		return fmt.Errorf("token cannot be empty")
	}

	fmt.Println()
	fmt.Println("Writing `./.substreams.env`")
	fmt.Println()
	fmt.Println("Please add `.substreams.env` to your `.gitignore`.")
	fmt.Println()

	err = os.WriteFile(".substreams.env", []byte(fmt.Sprintf("export SUBSTREAMS_API_TOKEN=%s\n", token)), 0644)
	if err != nil {
		return fmt.Errorf("writing .substreams.env file: %w", err)
	}

	fmt.Println("Load credentials in current terminal with:")
	color.Blue(" . ./.substreams.env")

	return nil
}
