package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(publishCmd)
	publishCmd.PersistentFlags().String("substreams-dev-endpoint", "http://localhost:9000/sf.substreams.dev.Api/PublishPackage", "Substreams dev endpoint")
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

	substreamsDevEndpoint, err := cmd.Flags().GetString("substreams-dev-endpoint")
	if err != nil {
		return err
	}

	var netTransport = &http.Transport{
		Dial: (&net.Dialer{
			Timeout: 5 * time.Second,
		}).Dial,
		TLSHandshakeTimeout: 5 * time.Second,
	}
	var httpClient = &http.Client{
		Timeout:   time.Second * 60,
		Transport: netTransport,
	}

	req, err := http.NewRequest("POST", substreamsDevEndpoint, requestBody)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
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
