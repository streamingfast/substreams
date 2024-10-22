package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/cobra"
)

func init() {
	publishCmd.PersistentFlags().String("registry", "https://api.substreams.dev", "Substreams dev endpoint")

	rootCmd.AddCommand(publishCmd)
}

var publishCmd = &cobra.Command{
	Use:   "publish <github_release_url>",
	Short: "Publish a package to the Substreams.dev registry",
	Args:  cobra.ExactArgs(1),
	RunE:  runPublish,
}

func runPublish(cmd *cobra.Command, args []string) error {
	githubReleaseUrl := args[0]

	org, err := getOrganizationFromGithubUrl(githubReleaseUrl)
	if err != nil {
		return err
	}

	request := &publishRequest{
		OrganizationSlug: slugify(org),
		GithubUrl:        githubReleaseUrl,
	}
	jsonRequest, _ := json.Marshal(request)
	requestBody := bytes.NewBuffer(jsonRequest)

	apiEndpoint, err := cmd.Flags().GetString("registry")
	if err != nil {
		return err
	}

	endpoint := fmt.Sprintf("%s/sf.substreams.dev.Api/PublishPackage", apiEndpoint)

	req, err := http.NewRequest("POST", endpoint, requestBody)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("failed to publish package: %s", resp.Status)
	}

	fmt.Println("Package published successfully")
	return nil
}

type publishRequest struct {
	OrganizationSlug string `json:"organization_slug"`
	GithubUrl        string `json:"github_url"`
}

func getOrganizationFromGithubUrl(url string) (string, error) {
	if !strings.Contains(url, "github.com") {
		return "", fmt.Errorf("invalid github url")
	}

	parts := strings.Split(url, "/")
	for i, part := range parts {
		if part == "github.com" && i < len(parts)-1 {
			return strings.ToLower(parts[i+1]), nil
		}
	}

	return "", fmt.Errorf("organization name not found in github url")
}

func slugify(s string) string {
	return strings.ReplaceAll(strings.ToLower(s), " ", "-")
}
