package templates

import (
	"bytes"
	"embed"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"github.com/iancoleman/strcase"
	"github.com/streamingfast/eth-go"
)

//go:embed ethereum/proto
//go:embed ethereum/src
//go:embed ethereum/build.rs
//go:embed ethereum/proto/contract.proto
//go:embed ethereum/Cargo.lock
//go:embed ethereum/Cargo.toml
//go:embed ethereum/substreams.yaml
//go:embed ethereum/rust-toolchain.toml
var ethereumProject embed.FS

type EthereumProject struct {
	name            string
	chain           *EthereumChain
	contractAddress eth.Address
	events          []codegenEvent
	abiContent      string
}

func NewEthereumProject(name string, chain *EthereumChain, address eth.Address, abi *eth.ABI, abiContent string) (*EthereumProject, error) {
	// We only have one templated file so far, so we can build own model correctly
	events, err := buildEventModels(abi)
	if err != nil {
		return nil, fmt.Errorf("build ABI event models: %w", err)
	}

	return &EthereumProject{
		name:            name,
		chain:           chain,
		contractAddress: address,
		events:          events,
		abiContent:      abiContent,
	}, nil
}

func (p *EthereumProject) Render() (map[string][]byte, error) {
	entries := map[string][]byte{}

	for _, ethereumProjectEntry := range []string{
		"proto/contract.proto",
		"src/abi/mod.rs",
		"src/pb/contract.v1.rs",
		"src/pb/mod.rs",
		"src/lib.rs.gotmpl",
		"build.rs",
		"Cargo.lock",
		"Cargo.toml",
		"substreams.yaml",
		"rust-toolchain.toml",
	} {
		content, err := ethereumProject.ReadFile(filepath.Join("ethereum", ethereumProjectEntry))
		if err != nil {
			return nil, fmt.Errorf("embed read entry %q: %w", ethereumProjectEntry, err)
		}

		finalFileName := ethereumProjectEntry

		if strings.HasSuffix(finalFileName, ".gotmpl") {
			tmpl, err := template.New(finalFileName).Parse(string(content))
			if err != nil {
				return nil, fmt.Errorf("embed parse entry template %q: %w", finalFileName, err)
			}

			buffer := bytes.NewBuffer(make([]byte, 0, uint64(float64(len(content))*1.10)))
			if err := tmpl.Execute(buffer, map[string]any{
				"name":        p.name,
				"module_name": p.name,
				"chain":       p.chain,
				"address":     p.contractAddress,
				"events":      p.events,
			}); err != nil {
				return nil, fmt.Errorf("embed render entry template %q: %w", finalFileName, err)
			}

			finalFileName = strings.TrimSuffix(finalFileName, ".gotmpl")
			content = buffer.Bytes()
		}

		entries[finalFileName] = content
	}

	entries["abi/contract.abi.json"] = []byte(p.abiContent)

	return entries, nil
}

type codegenEvent struct {
	RustName string
	Fields   map[string]string
}

func buildEventModels(abi *eth.ABI) (out []codegenEvent, err error) {
	names := keys(abi.LogEventsByNameMap)
	sort.StringSlice(names).Sort()

	// We allocate as much names + 16 to potentially account for duplicates
	out = make([]codegenEvent, 0, len(names)+16)
	for _, name := range names {
		events := abi.FindLogsByName(name)

		for i, event := range events {
			codegenEvent := codegenEvent{
				RustName: event.Name,
			}

			if len(events) > 1 {
				codegenEvent.RustName = name + strconv.FormatUint(uint64(i), 10)
			}

			if err := codegenEvent.populateFields(event); err != nil {
				return nil, fmt.Errorf("populating codegen fields: %w", err)
			}

			out = append(out, codegenEvent)
		}
	}

	return
}

func (e *codegenEvent) populateFields(log *eth.LogEventDef) error {
	if len(log.Parameters) == 0 {
		return nil
	}

	e.Fields = map[string]string{}
	for _, parameter := range log.Parameters {
		name := strcase.ToSnake(parameter.Name)

		var toJsonCode string
		switch v := parameter.Type.(type) {
		case
			eth.AddressType,
			eth.BytesType, eth.FixedSizeBytesType:
			toJsonCode = generateFieldTransformCode(v, "&event."+name)

		case
			eth.BooleanType,
			eth.StringType,
			eth.SignedIntegerType, eth.UnsignedIntegerType,
			eth.SignedFixedPointType, eth.UnsignedFixedPointType,
			eth.ArrayType:
			toJsonCode = generateFieldTransformCode(v, "event."+name)

		default:
			return fmt.Errorf("field type %q on parameter with name %q is not supported right now", parameter.TypeName, parameter.Name)
		}

		e.Fields[name] = toJsonCode
	}

	return nil
}

func generateFieldTransformCode(fieldType eth.SolidityType, fieldAccess string) string {
	switch v := fieldType.(type) {
	case eth.AddressType:
		return fmt.Sprintf("Hex(%s).to_string()", fieldAccess)

	case eth.BooleanType, eth.StringType:
		return fieldAccess

	case eth.BytesType, eth.FixedSizeBytesType:
		return fmt.Sprintf("Hex(%s).to_string()", fieldAccess)

	case eth.SignedIntegerType:
		if v.BitsSize <= 52 {
			return fmt.Sprintf("Into::<num_bigint::BigInt>::into(%s).to_i64().unwrap()", fieldAccess)
		}
		return fmt.Sprintf("%s.to_string()", fieldAccess)

	case eth.UnsignedIntegerType:
		if v.BitsSize <= 52 {
			return fmt.Sprintf("%s.to_u64()", fieldAccess)
		}
		return fmt.Sprintf("%s.to_string()", fieldAccess)

	case eth.SignedFixedPointType, eth.UnsignedFixedPointType:
		return fmt.Sprintf("%s.to_string()", fieldAccess)

	case eth.ArrayType:
		inner := generateFieldTransformCode(v.ElementType, "x")

		return fmt.Sprintf("%s.iter().map(|x| %s).collect::<Vec<_>>()", fieldAccess, inner)

	default:
		return ""
	}
}
