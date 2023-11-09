package templates

import (
	"bytes"
	"embed"
	"fmt"
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
//go:embed ethereum/substreams.sql.yaml.gotmpl
//go:embed ethereum/substreams.clickhouse.yaml.gotmpl
//go:embed ethereum/substreams.subgraph.yaml.gotmpl
//go:embed ethereum/rust-toolchain.toml
//go:embed ethereum/build.rs.gotmpl
//go:embed ethereum/schema.sql.gotmpl
//go:embed ethereum/schema.clickhouse.sql.gotmpl
//go:embed ethereum/schema.graphql.gotmpl
//go:embed ethereum/subgraph.yaml.gotmpl
var ethereumProject embed.FS

type EthereumContract struct {
	name       string
	address    eth.Address
	events     []codegenEvent
	abi        *eth.ABI
	abiContent string
}

func NewEthereumContract(name string, address eth.Address, events []codegenEvent, abi *eth.ABI, abiContent string) *EthereumContract {
	return &EthereumContract{
		name:       name,
		address:    address,
		events:     events,
		abi:        abi,
		abiContent: abiContent,
	}
}

func (e *EthereumContract) GetAddress() eth.Address {
	return e.address
}

func (e *EthereumContract) SetName(name string) {
	e.name = name
}

func (e *EthereumContract) GetName() string {
	return e.name
}

func (e *EthereumContract) SetEvents(events []codegenEvent) {
	e.events = events
}

func (e *EthereumContract) GetEvents() []codegenEvent {
	return e.events
}

func (e *EthereumContract) GetAbi() *eth.ABI {
	return e.abi
}

func (e *EthereumContract) SetAbi(abi *eth.ABI) {
	e.abi = abi
}

func (e *EthereumContract) SetAbiContent(abiContent string) {
	e.abiContent = abiContent
}

type EthereumProject struct {
	name                        string
	moduleName                  string
	chain                       *EthereumChain
	creationBlockNum            uint64
	ethereumContracts           []*EthereumContract
	sqlImportVersion            string
	graphImportVersion          string
	databaseChangeImportVersion string
	entityChangeImportVersion   string
	network                     string
}

func NewEthereumProject(name string, moduleName string, chain *EthereumChain, contracts []*EthereumContract, lowestStartBlock uint64) (*EthereumProject, error) {
	return &EthereumProject{
		name:                        name,
		moduleName:                  moduleName,
		chain:                       chain,
		ethereumContracts:           contracts,
		creationBlockNum:            lowestStartBlock,
		sqlImportVersion:            "1.0.5",
		graphImportVersion:          "0.1.0",
		databaseChangeImportVersion: "1.2.1",
		entityChangeImportVersion:   "1.1.0",
		network:                     chain.Network,
	}, nil
}

func (p *EthereumProject) Render() (map[string][]byte, error) {
	entries := map[string][]byte{}

	for _, ethereumProjectEntry := range []string{
		"proto/contract.proto.gotmpl",
		"src/abi/mod.rs.gotmpl",
		"src/pb/mod.rs",
		"src/lib.rs.gotmpl",
		"build.rs.gotmpl",
		"Cargo.lock",
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
		if ethereumProjectEntry == "src/lib.rs.gotmpl" && len(p.ethereumContracts) > 1 {
			ethereumProjectEntry = "src/multiple_contracts_lib.rs.gotmpl"
		}
		// We use directly "/" here as `ethereumProject` is an embed FS and always uses "/"
		content, err := ethereumProject.ReadFile("ethereum" + "/" + ethereumProjectEntry)
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

			name := p.name
			if finalFileName == "subgraph.yaml.gotmpl" {
				name = strcase.ToKebab(p.name)
			}

			model := map[string]any{
				"name":                        name,
				"moduleName":                  p.moduleName,
				"chain":                       p.chain,
				"ethereumContracts":           p.ethereumContracts,
				"initialBlock":                strconv.FormatUint(p.creationBlockNum, 10),
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

			if len(p.ethereumContracts) != 1 {
				finalFileName = strings.ReplaceAll(finalFileName, "multiple_contracts_", "")
			}

			finalFileName = strings.TrimSuffix(finalFileName, ".gotmpl")
			content = buffer.Bytes()
		}

		entries[finalFileName] = content
	}

	if len(p.ethereumContracts) == 1 {
		entries["abi/contract.abi.json"] = []byte(p.ethereumContracts[0].abiContent)
		return entries, nil
	}

	for _, contract := range p.ethereumContracts {
		entries[fmt.Sprintf("abi/%s_contract.abi.json", contract.GetName())] = []byte(contract.abiContent)
	}

	return entries, nil
}

func BuildEventModels(contract *EthereumContract, multipleContracts bool) (out []codegenEvent, err error) {
	abi := contract.abi
	pluralizer := pluralize.NewClient()

	names := keys(abi.LogEventsByNameMap)
	sort.StringSlice(names).Sort()

	// We allocate as many names + 16 to potentially account for duplicates
	out = make([]codegenEvent, 0, len(names)+16)
	for _, name := range names {
		events := abi.FindLogsByName(name)

		for i, event := range events {
			rustABIStructName := name
			if len(events) > 1 { // will result in OriginalName, OriginalName1, OriginalName2
				rustABIStructName = name + strconv.FormatUint(uint64(i+1), 10)
			}

			protoFieldName := strcase.ToSnake(pluralizer.Plural(rustABIStructName))
			// prost will do a to_lower_camel_case() on any struct name
			rustGeneratedStructName := strcase.ToCamel(strcase.ToSnake(rustABIStructName))

			codegenEvent := codegenEvent{
				Rust: &rustEventModel{
					ABIStructName:              rustGeneratedStructName,
					ProtoMessageName:           rustGeneratedStructName,
					ProtoOutputModuleFieldName: protoFieldName,
					TableChangeEntityName:      strcase.ToSnake(rustABIStructName),
				},

				Proto: &protoEventModel{
					MessageName:           rustGeneratedStructName,
					OutputModuleFieldName: protoFieldName,
				},
			}

			if err := codegenEvent.Rust.populateFields(event, multipleContracts); err != nil {
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
	ABIStructName              string
	ProtoMessageName           string
	ProtoOutputModuleFieldName string
	TableChangeEntityName      string
	ProtoFieldABIConversionMap map[string]string
	ProtoFieldTableChangesMap  map[string]string
	ProtoFieldSqlmap           map[string]string
	ProtoFieldClickhouseMap    map[string]string
	ProtoFieldGraphQLMap       map[string]string
}

func (e *rustEventModel) populateFields(log *eth.LogEventDef, multipleContracts bool) error {
	if len(log.Parameters) == 0 {
		return nil
	}

	e.ProtoFieldABIConversionMap = map[string]string{}
	e.ProtoFieldTableChangesMap = map[string]string{}
	e.ProtoFieldSqlmap = map[string]string{}
	e.ProtoFieldClickhouseMap = map[string]string{}
	e.ProtoFieldGraphQLMap = map[string]string{}
	paramNames := make([]string, len(log.Parameters))
	for i := range log.Parameters {
		paramNames[i] = log.Parameters[i].Name
	}
	fmt.Printf("  Generating ABI Events for %s (%s)\n", log.Name, strings.Join(paramNames, ","))

	for _, parameter := range log.Parameters {
		name := strcase.ToSnake(parameter.Name)
		name = sanitizeProtoFieldName(name)

		toProtoCode := generateFieldTransformCode(parameter.Type, "event."+name)
		if toProtoCode == SKIP_FIELD {
			continue
		}
		if toProtoCode == "" {
			return fmt.Errorf("transform - field type %q on parameter with name %q is not supported right now", parameter.TypeName, parameter.Name)
		}

		toDatabaseChangeCode := generateFieldTableChangeCode(parameter.Type, "evt."+name, multipleContracts)
		if toDatabaseChangeCode == SKIP_FIELD {
			continue
		}
		if toDatabaseChangeCode == "" {
			return fmt.Errorf("table change - field type %q on parameter with name %q is not supported right now", parameter.TypeName, parameter.Name)
		}

		toSqlCode := generateFieldSqlTypes(parameter.Type)
		if toSqlCode == SKIP_FIELD {
			continue
		}
		if toSqlCode == "" {
			return fmt.Errorf("sql - field type %q on parameter with name %q is not supported right now", parameter.TypeName, parameter.Name)
		}

		toClickhouseCode := generateFieldClickhouseTypes(parameter.Type)
		if toClickhouseCode == SKIP_FIELD {
			continue
		}
		if toClickhouseCode == "" {
			return fmt.Errorf("clickhouse - field type %q on parameter with name %q is not supported right now", parameter.TypeName, parameter.Name)
		}

		toGraphQLCode := generateFieldGraphQLTypes(parameter.Type)
		if toGraphQLCode == "" {
			return fmt.Errorf("graphql - field type %q on parameter with name %q is not supported right now", parameter.TypeName, parameter.Name)
		}

		columnName := sanitizeTableChangesColumnNames(name)

		e.ProtoFieldABIConversionMap[name] = toProtoCode
		e.ProtoFieldTableChangesMap[name] = toDatabaseChangeCode
		e.ProtoFieldSqlmap[columnName] = toSqlCode
		e.ProtoFieldClickhouseMap[columnName] = toClickhouseCode
		e.ProtoFieldGraphQLMap[name] = toGraphQLCode
	}

	return nil
}

func sanitizeProtoFieldName(name string) string {
	if strings.HasPrefix(name, "_") {
		return strings.Replace(name, "_", "u_", 1)
	}
	return name
}

func sanitizeTableChangesColumnNames(name string) string {
	return fmt.Sprintf("\"%s\"", name)
}

const SKIP_FIELD = "skip"

func generateFieldClickhouseTypes(fieldType eth.SolidityType) string {
	switch v := fieldType.(type) {
	case eth.AddressType:
		return "VARCHAR(40)"

	case eth.BooleanType:
		return "BOOL"

	case eth.BytesType, eth.FixedSizeBytesType, eth.StringType:
		return "TEXT"

	case eth.SignedIntegerType:
		switch {
		case v.BitsSize <= 8:
			return "Int8"
		case v.BitsSize <= 16:
			return "Int16"
		case v.BitsSize <= 32:
			return "Int32"
		case v.BitsSize <= 64:
			return "Int64"
		case v.BitsSize <= 128:
			return "Int128"
		}
		return "Int256"

	case eth.UnsignedIntegerType:
		switch {
		case v.BitsSize <= 8:
			return "UInt8"
		case v.BitsSize <= 16:
			return "UInt16"
		case v.BitsSize <= 32:
			return "UInt32"
		case v.BitsSize <= 64:
			return "UInt64"
		case v.BitsSize <= 128:
			return "UInt128"
		}
		return "UInt256"

	case eth.SignedFixedPointType:
		precision := v.Decimals
		if precision > 76 {
			precision = 76
		}
		switch {
		case v.BitsSize <= 32:
			return fmt.Sprintf("Decimal128(%d)", precision)
		case v.BitsSize <= 64:
			return fmt.Sprintf("Decimal128(%d)", precision)
		case v.BitsSize <= 128:
			return fmt.Sprintf("Decimal128(%d)", precision)
		}
		return fmt.Sprintf("Decimal256(%d)", precision)

	case eth.UnsignedFixedPointType:
		precision := v.Decimals
		if precision > 76 {
			precision = 76
		}
		switch {
		case v.BitsSize <= 31:
			return fmt.Sprintf("Decimal32(%d)", precision)
		case v.BitsSize <= 63:
			return fmt.Sprintf("Decimal64(%d)", precision)
		case v.BitsSize <= 127:
			return fmt.Sprintf("Decimal128(%d)", precision)
		}
		return fmt.Sprintf("Decimal256(%d)", precision)

	case eth.StructType:
		return SKIP_FIELD

		//case eth.ArrayType:
		//	elemType := generateFieldClickhouseTypes(v.ElementType)
		//	if elemType == "" || elemType == SKIP_FIELD {
		//		return elemType
		//	}
		//	return fmt.Sprintf("Array(%s)", elemType)

	default:
		return ""
	}
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

	case eth.StructType:
		return SKIP_FIELD

	//case eth.ArrayType:
	//	elemType := generateFieldClickhouseTypes(v.ElementType)
	//	if elemType == "" || elemType == SKIP_FIELD {
	//		return elemType
	//	}
	//	return fmt.Sprintf("%s ARRAY", elemType)

	default:
		return ""
	}
}

func generateFieldTableChangeCode(fieldType eth.SolidityType, fieldAccess string, multipleContract bool) string {
	switch v := fieldType.(type) {
	case eth.AddressType, eth.BytesType, eth.FixedSizeBytesType:
		return fmt.Sprintf("Hex(&%s).to_string()", fieldAccess)

	case eth.BooleanType:
		return fieldAccess

	case eth.StringType:
		if multipleContract {
			return fmt.Sprintf("&%s", fieldAccess)
		}
		return fieldAccess

	case eth.SignedIntegerType:
		if v.ByteSize <= 8 {
			return fieldAccess
		}
		return fmt.Sprintf("BigDecimal::from_str(&%s).unwrap()", fieldAccess)

	case eth.UnsignedIntegerType:
		if v.ByteSize <= 8 {
			return fieldAccess
		}
		return fmt.Sprintf("BigDecimal::from_str(&%s).unwrap()", fieldAccess)

	case eth.SignedFixedPointType, eth.UnsignedFixedPointType:
		return fmt.Sprintf("BigDecimal::from_str(&%s).unwrap()", fieldAccess)

	case eth.ArrayType:
		return SKIP_FIELD

	case eth.StructType:
		return SKIP_FIELD

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
		if inner == SKIP_FIELD {
			return SKIP_FIELD
		}
		return fmt.Sprintf("%s.into_iter().map(|x| %s).collect::<Vec<_>>()", fieldAccess, inner)

	case eth.StructType:
		return SKIP_FIELD

	default:
		return ""
	}
}

func generateFieldGraphQLTypes(fieldType eth.SolidityType) string {
	switch v := fieldType.(type) {
	case eth.AddressType:
		return "String!"

	case eth.BooleanType:
		return "Boolean!"

	case eth.BytesType, eth.FixedSizeBytesType, eth.StringType:
		return "String!"

	case eth.SignedIntegerType:
		if v.ByteSize <= 8 {
			return "Int!"
		}
		return "BigDecimal!"

	case eth.UnsignedIntegerType:
		if v.ByteSize <= 8 {
			return "Int!"
		}
		return "BigDecimal!"

	case eth.SignedFixedPointType, eth.UnsignedFixedPointType:
		return "BigDecimal!"

	case eth.StructType:
		return SKIP_FIELD

	case eth.ArrayType:
		return SKIP_FIELD

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

	e.Fields = make([]protoField, 0, len(log.Parameters))
	for _, parameter := range log.Parameters {
		fieldName := strcase.ToSnake(parameter.Name)
		fieldName = sanitizeProtoFieldName(fieldName)
		fieldType := getProtoFieldType(parameter.Type)
		if fieldType == SKIP_FIELD {
			continue
		}

		if fieldType == "" {
			return fmt.Errorf("field type %q on parameter with name %q is not supported right now", parameter.TypeName, parameter.Name)
		}

		e.Fields = append(e.Fields, protoField{Name: fieldName, Type: fieldType})
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
		fieldType := getProtoFieldType(v.ElementType)
		if fieldType == SKIP_FIELD {
			return SKIP_FIELD
		}
		return "repeated " + fieldType

	case eth.StructType:
		return SKIP_FIELD

	default:
		return ""
	}
}

type protoField struct {
	Name string
	Type string
}
