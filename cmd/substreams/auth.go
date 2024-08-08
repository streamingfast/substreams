package main

import (
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
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
	linkStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	if localDevelopment == "true" {
		fmt.Println(linkStyle.Render("http://localhost:3000/auth/substreams-devenv"))
	} else {
		fmt.Println(linkStyle.Render("https://thegraph.market/auth/substreams-devenv"))
	}

	var token string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				EchoMode(huh.EchoModePassword).
				Title("After retrieving your token, paste it here:").
				Inline(true).
				Value(&token).
				Validate(func(s string) error {
					if s == "" {
						return fmt.Errorf("token cannot be empty")
					}
					return nil
				}),
		),
	)

	fmt.Println("")
	if err := form.Run(); err != nil {
		return fmt.Errorf("error running form: %w", err)
	}

	fmt.Println("Writing `./.substreams.env`.  NOTE: Add it to `.gitignore`.")
	fmt.Println("")

	err := os.WriteFile(".substreams.env", []byte(fmt.Sprintf("export SUBSTREAMS_API_TOKEN=%s\n", token)), 0644)
	if err != nil {
		return fmt.Errorf("writing .substreams.env file: %w", err)
	}

	fmt.Println("Load credentials in current terminal with:")
	fmt.Println("")

	fmt.Println(linkStyle.Render("       . ./.substreams.env"))

	return nil
}
