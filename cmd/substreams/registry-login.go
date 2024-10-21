package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var registryLoginCmd = &cobra.Command{
	Use:          "login",
	Short:        "Login to the Substreams registry",
	SilenceUsage: true,
	RunE:         runRegistryLoginE,
}

var registryFileName = "registry-token"

func init() {
	registryLoginCmd.Flags().String("registry", "https://api.substreams.dev", "Substreams registry URL")

	registryCmd.AddCommand(registryLoginCmd)
}

func runRegistryLoginE(cmd *cobra.Command, args []string) error {
	registryURL, err := cmd.Flags().GetString("registry")
	if err != nil {
		return fmt.Errorf("could not get registry URL: %w", err)
	}

	loginRegistryPage := fmt.Sprintf("%s/me", registryURL)

	fmt.Printf("Paste the token found on %s below\n", loginRegistryPage)

	scanner := bufio.NewScanner(os.Stdin)
	var token string
	for scanner.Scan() {
		token = scanner.Text()
		break
	}

	fmt.Println("")

	isFileExists := checkFileExists(registryFileName)
	if isFileExists {
		fmt.Println("Token already saved to registry-token")
		fmt.Printf("Do you want to overwrite it? [y/N] ")
		scanner.Scan()
		if scanner.Text() == "y" {
			err = writeRegistryToken(token)
			if err != nil {
				return fmt.Errorf("could not write token to registry: %w", err)
			}
		} else {
			return nil
		}

	} else {
		err = writeRegistryToken(token)
		if err != nil {
			return fmt.Errorf("could not write token to registry: %w", err)
		}

	}

	fmt.Printf("Publish packages with SUBSTREAMS_REGISTRY_TOKEN=%s\n", token)
	fmt.Printf("Token %s saved to registry-token\n", token)
	return nil
}

func writeRegistryToken(token string) error {
	token = fmt.Sprintf("SUBSTREAMS_REGISTRY_TOKEN=%s", token)
	return os.WriteFile(registryFileName, []byte(token), 0644)
}

func checkFileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return !errors.Is(err, os.ErrNotExist)
}
