package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/streamingfast/cli"
	"github.com/streamingfast/cli/utils"
	"github.com/streamingfast/substreams/manifest"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

var registryPublish = &cobra.Command{
	Use:   "publish [github_release_url | https_spkg_path | local_spkg_path | local_substreams_path]",
	Short: "Publish a package to the Substreams.dev registry",
	Long: cli.Dedent(`
		Publish a package to the Substreams.dev registry. You can specify a GitHub release URL, HTTPS spkg path,
		local spkg path, or local substreams path. If no argument is provided, it will look for a substreams.yaml
		file in the current directory. You can use "-" to read the manifest from standard input.
	`),
	Args: cobra.MaximumNArgs(1),
	RunE: runRegistryPublish,
}

var teamSlug string

func init() {
	registryPublish.Flags().StringVarP(&teamSlug, "teamSlug", "t", "", "Team slug to publish the package under (e.g. 'myteam' instead of 'My Team')")
	registryCmd.AddCommand(registryPublish)
}

func runRegistryPublish(cmd *cobra.Command, args []string) (err error) {
	apiEndpoint := getSubstreamsRegistryEndpoint()

	token, err := getRegistryToken(apiEndpoint)
	if err != nil {
		return fmt.Errorf("getting registry token: %w", err)
	}

	zlog.Debug("loaded api key", zap.String("token", token))

	var manifestPath string
	switch len(args) {
	case 0:
		manifestPath, err = resolveManifestFile("")
		if err != nil {
			return fmt.Errorf("resolving manifest: %w", err)
		}
	case 1:
		manifestPath = args[0]
	}

	spkgRegistry := getSubstreamsDownloadEndpoint()

	readerOptions := []manifest.Option{
		manifest.WithRegistryURL(spkgRegistry),
	}

	manifestReader, err := manifest.NewReader(manifestPath, readerOptions...)
	if err != nil {
		return fmt.Errorf("manifest reader: %w", err)
	}

	pkgBundle, err := manifestReader.Read()
	if err != nil {
		return fmt.Errorf("read manifest %q: %w", manifestPath, err)
	}

	spkg := pkgBundle.Package

	// Validate proto output types
	if err := manifest.ValidateProtoOutputTypes(spkg); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	warnIncompletePackage(spkg, warningsConfig{})
	printPackageDetails(spkg)

	confirm, err := utils.RunConfirmForm("Would you like to publish this package?")
	if err != nil {
		return fmt.Errorf("running confirm form %w", err)
	}

	if !confirm {
		return nil
	}

	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)

	// Create form file to get it read from the `substreams.dev`  server
	formFile, err := writer.CreateFormFile("file", "substreams_package")
	if err != nil {
		return fmt.Errorf("failed to create form file: %w", err)
	}

	data, err := proto.Marshal(spkg)
	if err != nil {
		return fmt.Errorf("marshalling substreams package: %w", err)
	}

	_, err = formFile.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write file content: %w", err)
	}

	// Add team slug to form data if provided
	if teamSlug != "" {
		err = writer.WriteField("teamSlug", teamSlug)
		if err != nil {
			return fmt.Errorf("failed to write team slug field: %w", err)
		}
	}

	err = writer.Close()
	if err != nil {
		return fmt.Errorf("failed to close writer: %w", err)
	}

	publishPackageEndpoint := fmt.Sprintf("%s/sf.substreams.dev.Api/PublishPackage", apiEndpoint)

	zlog.Debug("publishing package", zap.String("registry_url", publishPackageEndpoint))

	req, err := http.NewRequest("POST", publishPackageEndpoint, &requestBody)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Api-Key", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read body")
	}

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusUnauthorized {
			fmt.Println("")
			fmt.Println(cli.ErrorStyle.Render("Failed to publish package"))
			fmt.Println(cli.ErrorStyle.Render("Reason: " + string(b)))
			fmt.Println("Make sure you are properly authenticated with:")
			fmt.Println("")
			fmt.Println(cli.PurpleStyle.Render("substreams registry login"))
			return nil
		}

		fmt.Println("")
		fmt.Println(cli.ErrorStyle.Render("Failed to publish package"))
		fmt.Println(cli.ErrorStyle.Render("Reason: " + string(b)))
		return nil
	}

	registryURL := gjson.Get(string(b), "registry_url").String()
	packageName := gjson.Get(string(b), "package_name").String()
	releaseVersion := gjson.Get(string(b), "release_version").String()

	fmt.Println("Package published successfully!")
	fmt.Println("")
	fmt.Println("View on the web at: ", slugifyPackageName(registryURL))
	fmt.Println("")
	fmt.Println("Start streaming with:")
	fmt.Println("")
	fmt.Printf("    substreams gui %s@%s\n", slugifyPackageName(packageName), releaseVersion)
	fmt.Println("")

	return nil
}

func getRegistryToken(apiEndpoint string) (string, error) {
	token := os.Getenv("SUBSTREAMS_REGISTRY_TOKEN")
	if token == "" {
		registryTokenBytes, err := os.ReadFile(registryTokenFilename)
		if err != nil {
			if !os.IsNotExist(err) {
				return "", fmt.Errorf("failed to read registry token: %w", err)
			}
		}

		token = strings.TrimSpace(string(registryTokenBytes))

		if token == "" {
			fmt.Println("No registry token found...")
			fmt.Println()
			linkStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
			inputtedToken, err := copyPasteTokenForm(apiEndpoint, linkStyle)
			if err != nil {
				return "", fmt.Errorf("creating copy, paste token form %w", err)
			}

			token = inputtedToken
		}
	}
	return token, nil
}

func copyPasteTokenForm(endpoint string, linkStyle lipgloss.Style) (string, error) {
	fmt.Printf("Login to the Substreams registry.")
	fmt.Println()
	fmt.Println()
	fmt.Println("Navigate to: ")
	fmt.Println()
	fmt.Println("    " + linkStyle.Render(fmt.Sprintf("%s/me", endpoint)))
	fmt.Println("")

	var token string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				EchoMode(huh.EchoModePassword).
				Title("Paste the token here:").
				Inline(true).
				Value(&token).
				Validate(func(s string) error {
					if s == "" {
						return errors.New("token cannot be empty")
					}
					return nil
				}),
		),
	)

	if err := form.Run(); err != nil {
		return "", fmt.Errorf("error running form: %w", err)
	}

	return token, nil
}

func slugifyPackageName(s string) (slug string) {
	slug = strings.Replace(s, "_", "-", -1)
	return
}
