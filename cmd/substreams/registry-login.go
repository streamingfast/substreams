package main

import (
	"errors"
	"fmt"
	"github.com/charmbracelet/lipgloss"
	"github.com/streamingfast/cli/utils"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var registryLoginCmd = &cobra.Command{
	Use:          "login",
	Short:        "Login to the Substreams registry",
	SilenceUsage: true,
	RunE:         runRegistryLoginE,
}

var registryTokenFilename = filepath.Join(os.Getenv("HOME"), ".config", "substreams", "registry-token")

func init() {
	registryCmd.AddCommand(registryLoginCmd)
}

func runRegistryLoginE(cmd *cobra.Command, args []string) error {
	registryURL := getSubstreamsRegistryEndpoint()

	linkStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	token, err := copyPasteTokenForm(registryURL, linkStyle)
	if err != nil {
		return fmt.Errorf("creating copy, paste token form %w", err)
	}

	isFileExists := checkFileExists(registryTokenFilename)
	if isFileExists {
		confirmOverwrite, err := utils.RunConfirmForm("Token already saved to ~/.config/substreams/registry-token, do you want to overwrite it?")
		if err != nil {
			return fmt.Errorf("running confirm form: %w", err)
		}

		if confirmOverwrite {
			err := writeRegistryToken(token)
			if err != nil {
				return fmt.Errorf("could not write token to registry: %w", err)
			}
		} else {
			return nil
		}

	} else {
		err := writeRegistryToken(token)
		if err != nil {
			return fmt.Errorf("could not write token to registry: %w", err)
		}

	}

	fmt.Printf("All set! Token written to ~/.config/substreams/registry-token\n")
	fmt.Println()

	return nil
}

func writeRegistryToken(token string) error {
	return os.WriteFile(registryTokenFilename, []byte(token), 0644)
}

func checkFileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return !errors.Is(err, os.ErrNotExist)
}
