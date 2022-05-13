package tools

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
)

var packageCmd = &cobra.Command{
	Use:   "package",
	Short: "subcommand for managing substreams packages",
}

func init() {
	Cmd.AddCommand(packageCmd)
	packageCmd.AddCommand(pkgInfoCmd)
	packageCmd.AddCommand(pkgCreateCmd)
	packageCmd.AddCommand(pkgGetCmd)
	packageCmd.AddCommand(pkgListCmd)
}

var pkgInfoCmd = &cobra.Command{
	Use:   "info",
	Short: "Prints information about current package (and validate)",
	RunE:  pkgInfoE,
}

func pkgInfoE(cmd *cobra.Command, _ []string) error {
	c, err := readCargo("Cargo.toml")
	if err != nil {
		return err
	}
	fmt.Printf("Information from Cargo.toml: %+v\n", c)

	files, err := requiredFiles(c)
	if err != nil {
		return err
	}
	fmt.Println("List of files that would be bundled:", files)
	return nil
}

var pkgCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Creates a package (tar.gz)",
	RunE:  pkgCreateE,
}

func pkgCreateE(cmd *cobra.Command, _ []string) error {
	c, err := readCargo("Cargo.toml")
	if err != nil {
		return err
	}
	fmt.Printf("Creating package %s (version %s)", c.Package.Name, c.Package.Version)

	files, err := requiredFiles(c)
	if err != nil {
		return err
	}

	filename := fmt.Sprintf("%s-%s.tar.gz", packageNameInFile(c.Package.Name), c.Package.Version)
	out, err := os.Create(filename)
	if err != nil {
		return err
	}

	if err := CreateArchive(files, out); err != nil {
		return fmt.Errorf("cannot create package %s: %w", filename, err)
	}
	fmt.Printf("Wrote to file: %s\n", filename)
	fmt.Println("You should now upload this file to a github release so it can be fetched")
	return nil
}

var pkgGetCmd = &cobra.Command{
	Use:   "get {name}@{version} {source}",
	Short: "Download package from source, extract in './packages'",
	Long:  "{name} is something like 'substreams-erc20'\nversion is something like '1.2.3'\ncurrently supported sources are 'github:owner/repo' or 'https://example.com/path/to/filename.tar.gz'",
	Args:  cobra.ExactArgs(2),
	RunE:  pkgGetE,
}

func pkgGetE(cmd *cobra.Command, args []string) error {

	name, version, err := parsePkgNameVersion(args[0])
	source := args[1]

	dest := filepath.Join("packages", name)
	if err := checkFileExists(dest); err == nil { // IT EXISTS
		return fmt.Errorf("cannot install package %q, it is already present at %q (remove it first)", name, dest)
	}

	url := source
	if strings.HasPrefix(source, "github:") {
		u, err := resolveGithubURL(name, version, source)
		if err != nil {
			return fmt.Errorf("resolving github URL: %w", err)
		}
		url = u
	}

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("getting package from %q: %w", url, err)
	}
	if resp.StatusCode > http.StatusOK {
		return fmt.Errorf("getting package from %q: %s", url, resp.Status)

	}
	defer resp.Body.Close()
	if err := Untar(dest, resp.Body); err != nil {
		return fmt.Errorf("untar package file into %q: %w", dest, err)
	}
	fmt.Printf("downloaded package under %s\n", dest)
	return nil
}

var pkgListCmd = &cobra.Command{
	Use:   "list",
	Short: "list packages present under './packages' and their versions",
	RunE:  pkgListE,
}

func pkgListE(cmd *cobra.Command, _ []string) error {

	packages, _ := filepath.Glob("packages/*")
	for _, pkg := range packages {
		cargoFile := filepath.Join(pkg, "Cargo.toml")
		def, err := readCargo(cargoFile)
		if err != nil {
			return fmt.Errorf("error with cargo file in package %q: %w", pkg, err)
		}
		fmt.Printf("- %s (%s)\n", def.Package.Name, def.Package.Version)
	}

	return nil
}

// func CreateArchive(files []string, buf io.Writer) error {
type CargoDefPackage struct {
	Name        string
	Version     string
	Description string
	Edition     string
}

type CargoDef struct {
	Package           CargoDefPackage
	Dependencies      map[string]interface{}
	BuildDependencies map[string]interface{} `toml:"build-dependencies"`
}

func readCargo(filename string) (*CargoDef, error) {
	in, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	out := &CargoDef{}
	err = toml.Unmarshal(in, &out)
	return out, err

}

func validateCargo(c *CargoDef) error {
	if c.Package.Name == "" {
		return fmt.Errorf("invalid package name")
	}
	if c.Package.Version == "" {
		return fmt.Errorf("invalid package version")
	}
	return nil
}

func packageNameInFile(in string) string {
	return strings.Replace(in, "-", "_", -1)
}

// requiredFiles determines all the files needed and returns an error if any of them is missing
func requiredFiles(cargo *CargoDef) ([]string, error) {
	if err := validateCargo(cargo); err != nil {
		return nil, err
	}

	wasmFile := fmt.Sprintf("%s.wasm", packageNameInFile(cargo.Package.Name))
	cargoFile := "Cargo.toml"
	yamlFile := "substreams.yaml"
	out := []string{cargoFile, wasmFile, yamlFile}

	for _, f := range out {
		if err := checkFileExists(f); err != nil {
			return nil, err
		}
	}

	protos, _ := filepath.Glob("**/*.proto")
	out = append(out, protos...)

	rustPBs, _ := filepath.Glob("src/pb/*.rs")
	out = append(out, rustPBs...)

	return out, nil
}

func checkFileExists(filename string) error {
	if _, err := os.Stat(filename); err != nil {
		return fmt.Errorf("file %q not found or cannot be read: %w", filename, err)
	}
	return nil
}

func parsePkgNameVersion(in string) (string, string, error) {
	parts := strings.Split(in, "@")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid 'name@version' string")
	}

	return parts[0], parts[1], nil
}
func resolveGithubURL(name, version, source string) (string, error) {
	parts := strings.Split(source[7:], "/")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid github source, must be of format: 'github:owner/repo'")
	}
	return fmt.Sprintf("https://github.com/%s/%s/releases/download/v%s/%s-%s.tar.gz", parts[0], parts[1], version, packageNameInFile(name), version), nil
}
