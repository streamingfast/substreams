package templates

import (
	"bytes"
	"embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/huandu/xstrings"
	"go.uber.org/zap"
)

//go:embed starknet/.gitignore
//go:embed starknet/proto/block.proto.gotmpl
//go:embed starknet/src/pb/mod.rs
//go:embed starknet/src/lib.rs.gotmpl
//go:embed starknet/Cargo.toml.gotmpl
//go:embed starknet/Makefile.gotmpl
//go:embed starknet/substreams.yaml.gotmpl
//go:embed starknet/substreams.sql.yaml.gotmpl
//go:embed starknet/substreams.clickhouse.yaml.gotmpl
//go:embed starknet/substreams.subgraph.yaml.gotmpl
//go:embed starknet/rust-toolchain.toml
//go:embed starknet/schema.sql.gotmpl
//go:embed starknet/schema.clickhouse.sql.gotmpl
//go:embed starknet/schema.graphql.gotmpl
//go:embed starknet/subgraph.yaml.gotmpl
var starknetProject embed.FS

type StarknetProject struct {
	name                        string
	moduleName                  string
	chain                       *StarknetChain
	sqlImportVersion            string
	graphImportVersion          string
	databaseChangeImportVersion string
	entityChangeImportVersion   string
	network                     string
}

func NewStarknetProject(name string, moduleName string, chain *StarknetChain) (*StarknetProject, error) {
	return &StarknetProject{
		name:                        name,
		moduleName:                  moduleName,
		chain:                       chain,
		sqlImportVersion:            "1.0.7",
		graphImportVersion:          "0.1.0",
		databaseChangeImportVersion: "1.2.1",
		entityChangeImportVersion:   "1.1.0",
		network:                     chain.Network,
	}, nil
}

func (p *StarknetProject) Render() (map[string][]byte, error) {
	entries := map[string][]byte{}

	for _, starknetProjectEntry := range []string{
		".gitignore",
		"proto/block.proto.gotmpl",
		"src/pb/mod.rs",
		"src/lib.rs.gotmpl",
		"Cargo.toml.gotmpl",
		"Makefile.gotmpl",
		"substreams.yaml.gotmpl",
		"substreams.sql.yaml.gotmpl",
		"substreams.clickhouse.yaml.gotmpl",
		"substreams.subgraph.yaml.gotmpl",
		"rust-toolchain.toml",
		"schema.sql.gotmpl",
		"schema.clickhouse.sql.gotmpl",
		"schema.graphql.gotmpl",
		"subgraph.yaml.gotmpl",
	} {
		// We use directly "/" here as `starknetProject` is an embed FS and always uses "/"
		content, err := starknetProject.ReadFile("starknet" + "/" + starknetProjectEntry)
		if err != nil {
			return nil, fmt.Errorf("embed read entry %q: %w", starknetProjectEntry, err)
		}

		finalFileName := starknetProjectEntry

		zlog.Debug("reading starknet project entry", zap.String("filename", finalFileName))

		if strings.HasSuffix(finalFileName, ".gotmpl") {
			tmpl, err := template.New(finalFileName).Funcs(ProjectGeneratorFuncs).Parse(string(content))
			if err != nil {
				return nil, fmt.Errorf("embed parse entry template %q: %w", finalFileName, err)
			}

			name := p.name
			if finalFileName == "subgraph.yaml.gotmpl" {
				name = xstrings.ToKebabCase(p.name)
			}

			model := map[string]any{
				"name":                        name,
				"moduleName":                  p.moduleName,
				"chain":                       p.chain,
				"initialBlock":                0,
				"sqlImportVersion":            p.sqlImportVersion,
				"graphImportVersion":          p.graphImportVersion,
				"databaseChangeImportVersion": p.databaseChangeImportVersion,
				"entityChangeImportVersion":   p.entityChangeImportVersion,
				"network":                     p.network,
			}

			zlog.Debug("rendering templated file", zap.String("filename", finalFileName), zap.Any("model", model))

			buffer := bytes.NewBuffer(make([]byte, 0, uint64(float64(len(content))*1.10)))
			if err := tmpl.Execute(buffer, model); err != nil {
				return nil, fmt.Errorf("embed render entry template %q: %w", finalFileName, err)
			}

			finalFileName = strings.TrimSuffix(finalFileName, ".gotmpl")
			content = buffer.Bytes()
		}

		entries[finalFileName] = content
	}

	return entries, nil
}
