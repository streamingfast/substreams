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

	"github.com/gertd/go-pluralize"
	"github.com/iancoleman/strcase"
	"github.com/streamingfast/eth-go"
	"go.uber.org/zap"
)

//go:embed ethereum/proto
//go:embed ethereum/src
//go:embed ethereum/build.rs
//go:embed ethereum/Cargo.lock
//go:embed ethereum/Cargo.toml.gotmpl
//go:embed ethereum/Makefile.gotmpl
//go:embed ethereum/substreams.yaml.gotmpl
//go:embed ethereum/rust-toolchain.toml
//go:embed ethereum/schema.sql.gotmpl
var ethereumProject embed.FS

type EthereumProject struct {
	name                        string
	moduleName                  string
	chain                       *EthereumChain
	contractAddress             eth.Address
	events                      []codegenEvent
	abiContent                  string
	creationBlockNum            uint64
	sqlImportVersion            string
	databaseChangeImportVersion string
	network                     string
}

func NewEthereumProject(name string, moduleName string, chain *EthereumChain, address eth.Address, abi *eth.ABI, abiContent string, creationBlockNum uint64) (*EthereumProject, error) {
	// We only have one templated file so far, so we can build own model correctly
	events, err := buildEventModels(abi)
	if err != nil {
		return nil, fmt.Errorf("build ABI event models: %w", err)
	}

	return &EthereumProject{
		name:                        name,
		moduleName:                  moduleName,
		chain:                       chain,
		contractAddress:             address,
		events:                      events,
		abiContent:                  abiContent,
		creationBlockNum:            creationBlockNum,
		sqlImportVersion:            "1.0.2",
		databaseChangeImportVersion: "1.2.1",
		network:                     chain.Network,
	}, nil
}

func (p *EthereumProject) Render() (map[string][]byte, error) {
	entries := map[string][]byte{}

	for _, ethereumProjectEntry := range []string{
		"proto/contract.proto.gotmpl",
		"src/abi/mod.rs",
		"src/pb/contract.v1.rs",
		"src/pb/mod.rs",
		"src/lib.rs.gotmpl",
		"build.rs",
		"Cargo.lock",
		"Cargo.toml.gotmpl",
		"Makefile.gotmpl",
		"substreams.yaml.gotmpl",
		"rust-toolchain.toml",
		"schema.sql.gotmpl",
	} {
		content, err := ethereumProject.ReadFile(filepath.Join("ethereum", ethereumProjectEntry))
		if err != nil {
			return nil, fmt.Errorf("embed read entry %q: %w", ethereumProjectEntry, err)
		}

		finalFileName := ethereumProjectEntry
		zlog.Debug("reading ethereum project entry", zap.String("filename", finalFileName))

		if strings.HasSuffix(finalFileName, ".gotmpl") {
			tmpl, err := template.New(finalFileName).Funcs(ProjectGeneratorFuncs).Parse(string(content))
			if err != nil {
				return nil, fmt.Errorf("embed parse entry template %q: %w", finalFileName, err)
			}

			model := map[string]any{
				"name":                        p.name,
				"moduleName":                  p.moduleName,
				"chain":                       p.chain,
				"address":                     p.contractAddress,
				"events":                      p.events,
				"initialBlock":                strconv.FormatUint(p.creationBlockNum, 10),
				"sqlImportVersion":            p.sqlImportVersion,
				"databaseChangeImportVersion": p.databaseChangeImportVersion,
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

	entries["abi/contract.abi.json"] = []byte(p.abiContent)

	return entries, nil
}

func buildEventModels(abi *eth.ABI) (out []codegenEvent, err error) {
	pluralizer := pluralize.NewClient()

	names := keys(abi.LogEventsByNameMap)
	sort.StringSlice(names).Sort()

	// We allocate as many names + 16 to potentially account for duplicates
	out = make([]codegenEvent, 0, len(names)+16)
	for _, name := range names {
		events := abi.FindLogsByName(name)

		for i, event := range events {
			rustABIStructName := name
			if len(events) > 1 {
				rustABIStructName = name + strconv.FormatUint(uint64(i), 10)
			}

			protoFieldName := strcase.ToSnake(pluralizer.Plural(rustABIStructName))

			codegenEvent := codegenEvent{
				Rust: &rustEventModel{
					ABIStructName:              rustABIStructName,
					ProtoMessageName:           rustABIStructName,
					ProtoOutputModuleFieldName: protoFieldName,
				},

				Proto: &protoEventModel{
					MessageName:           rustABIStructName,
					OutputModuleFieldName: protoFieldName,
				},
			}

			if err := codegenEvent.Rust.populateFields(event); err != nil {
				return nil, fmt.Errorf("populating codegen Rust fields: %w", err)
			}

			if err := codegenEvent.Proto.populateFields(event); err != nil {
				return nil, fmt.Errorf("populating codegen Proto fields: %w", err)
			}

			out = append(out, codegenEvent)
		}
	}

	return
}

type codegenEvent struct {
	Rust  *rustEventModel
	Proto *protoEventModel
}

type rustEventModel struct {
	ABIStructName                string
	ProtoMessageName             string
	ProtoOutputModuleFieldName   string
	ProtoFieldABIConversionMap   map[string]string
	ProtoFieldDatabaseChangesMap map[string]string
	ProtoFieldSqlmap             map[string]string
}

func (e *rustEventModel) populateFields(log *eth.LogEventDef) error {
	if len(log.Parameters) == 0 {
		return nil
	}

	e.ProtoFieldABIConversionMap = map[string]string{}
	e.ProtoFieldDatabaseChangesMap = map[string]string{}
	e.ProtoFieldSqlmap = map[string]string{}
	paramNames := make([]string, len(log.Parameters))
	for i := range log.Parameters {
		paramNames[i] = log.Parameters[i].Name
	}
	fmt.Printf("  Generating ABI Events for %s (%s)\n", log.Name, strings.Join(paramNames, ","))

	for _, parameter := range log.Parameters {
		name := strcase.ToSnake(parameter.Name)
		name = sanitizeProtoFieldName(name)

		toProtoCode := generateFieldTransformCode(parameter.Type, "event."+name)
		if toProtoCode == "" {
			return fmt.Errorf("field type %q on parameter with name %q is not supported right now", parameter.TypeName, parameter.Name)
		}

		toDatabaseChangeCode := generateFieldDatabaseChangeCode(parameter.Type, "evt."+name)
		if toDatabaseChangeCode == "" {
			return fmt.Errorf("field type %q on parameter with name %q is not supported right now", parameter.TypeName, parameter.Name)
		}

		toSqlCode := generateFieldSqlTypes(parameter.Type)
		if toSqlCode == "" {
			return fmt.Errorf("field type %q on parameter with name %q is not supported right now", parameter.TypeName, parameter.Name)
		}

		columnName := sanitizeDatabaseChangesColumnNames(name)

		e.ProtoFieldABIConversionMap[name] = toProtoCode
		e.ProtoFieldDatabaseChangesMap[name] = toDatabaseChangeCode
		e.ProtoFieldSqlmap[columnName] = toSqlCode
	}

	return nil
}

func sanitizeProtoFieldName(name string) string {
	if strings.HasPrefix(name, "_") {
		return strings.Replace(name, "_", "u_", 1)
	}
	return name
}

func sanitizeDatabaseChangesColumnNames(name string) string {
	return fmt.Sprintf("\"%s\"", name)
}

func generateFieldSqlTypes(fieldType eth.SolidityType) string {
	switch v := fieldType.(type) {
	case eth.AddressType:
		return "VARCHAR(40)"

	case eth.BooleanType:
		return "BOOL"

	case eth.BytesType, eth.FixedSizeBytesType, eth.StringType:
		return "TEXT"

	case eth.SignedIntegerType:
		if v.ByteSize <= 8 {
			return "INT"
		}
		return "DECIMAL"

	case eth.UnsignedIntegerType:
		if v.ByteSize <= 8 {
			return "INT"
		}
		return "DECIMAL"

	case eth.SignedFixedPointType, eth.UnsignedFixedPointType:
		return "DECIMAL"

	case eth.ArrayType:
		return "" // not currently supported

	default:
		return ""
	}
}

func generateFieldDatabaseChangeCode(fieldType eth.SolidityType, fieldAccess string) string {
	switch v := fieldType.(type) {
	case eth.AddressType, eth.BytesType, eth.FixedSizeBytesType:
		return fmt.Sprintf("Hex(&%s).to_string()", fieldAccess)

	case eth.BooleanType, eth.StringType:
		return fieldAccess

	case eth.SignedIntegerType:
		if v.ByteSize <= 8 {
			return fieldAccess
		}
		return fmt.Sprintf("%s.to_string()", fieldAccess)

	case eth.UnsignedIntegerType:
		if v.ByteSize <= 8 {
			return fieldAccess
		}
		return fmt.Sprintf("%s.to_string()", fieldAccess)

	case eth.SignedFixedPointType, eth.UnsignedFixedPointType:
		return fmt.Sprintf("%s.to_string()", fieldAccess)

	case eth.ArrayType:
		inner := generateFieldTransformCode(v.ElementType, "x")
		return fmt.Sprintf("%s.into_iter().map(|x| %s).collect::<Vec<_>>()", fieldAccess, inner)

	default:
		return ""
	}
}

func generateFieldTransformCode(fieldType eth.SolidityType, fieldAccess string) string {
	switch v := fieldType.(type) {
	case eth.AddressType:
		return fieldAccess

	case eth.BooleanType, eth.StringType:
		return fieldAccess

	case eth.BytesType:
		return fieldAccess

	case eth.FixedSizeBytesType:
		return fmt.Sprintf("Vec::from(%s)", fieldAccess)

	case eth.SignedIntegerType:
		if v.ByteSize <= 8 {
			return fmt.Sprintf("Into::<num_bigint::BigInt>::into(%s).to_i64().unwrap()", fieldAccess)
		}
		return fmt.Sprintf("%s.to_string()", fieldAccess)

	case eth.UnsignedIntegerType:
		if v.ByteSize <= 8 {
			return fmt.Sprintf("%s.to_u64()", fieldAccess)
		}
		return fmt.Sprintf("%s.to_string()", fieldAccess)

	case eth.SignedFixedPointType, eth.UnsignedFixedPointType:
		return fmt.Sprintf("%s.to_string()", fieldAccess)

	case eth.ArrayType:
		inner := generateFieldTransformCode(v.ElementType, "x")
		return fmt.Sprintf("%s.into_iter().map(|x| %s).collect::<Vec<_>>()", fieldAccess, inner)

	default:
		return ""
	}
}

type protoEventModel struct {
	// MessageName is the name of the message representing this specific event
	MessageName string

	OutputModuleFieldName string
	Fields                []protoField
}

func (e *protoEventModel) populateFields(log *eth.LogEventDef) error {
	if len(log.Parameters) == 0 {
		return nil
	}

	e.Fields = make([]protoField, len(log.Parameters))
	for index, parameter := range log.Parameters {
		fieldName := strcase.ToSnake(parameter.Name)
		fieldName = sanitizeProtoFieldName(fieldName)
		fieldType := getProtoFieldType(parameter.Type)

		if fieldType == "" {
			return fmt.Errorf("field type %q on parameter with name %q is not supported right now", parameter.TypeName, parameter.Name)
		}

		e.Fields[index] = protoField{Name: fieldName, Type: fieldType}
	}

	return nil
}

func getProtoFieldType(solidityType eth.SolidityType) string {
	switch v := solidityType.(type) {
	case eth.AddressType, eth.BytesType, eth.FixedSizeBytesType:
		return "bytes"

	case eth.BooleanType:
		return "bool"

	case eth.StringType:
		return "string"

	case eth.SignedIntegerType:
		if v.ByteSize <= 8 {
			return "int64"
		}

		return "string"

	case eth.UnsignedIntegerType:
		if v.ByteSize <= 8 {
			return "uint64"
		}

		return "string"

	case eth.SignedFixedPointType, eth.UnsignedFixedPointType:
		return "string"

	case eth.ArrayType:
		// Flaky, I think we should support a single level of "array"
		return "repeated " + getProtoFieldType(v.ElementType)

	default:
		return ""
	}
}

type protoField struct {
	Name string
	Type string
}
