package codegen

import (
	"encoding/json"
	"fmt"
	"github.com/iancoleman/strcase"
	"github.com/streamingfast/eth-go"
	"io/ioutil"
	"net/http"
	"sort"
	"strconv"
)

type CodegenEvent struct {
	RustName string
	Fields   map[string]string
}

func GetContractAbi(contract string) ([]byte, error) {
	res, err := http.Get(fmt.Sprintf("https://api.etherscan.io/api?module=contract&action=getabi&address=%s&apikey=7E11P1IJ4ZRWQ36CZN78QZ7KTE7YP7MJ31", contract))
	if err != nil {
		return nil, fmt.Errorf("getting contract abi from etherscan: %w", err)
	}
	defer res.Body.Close()

	abi, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("reading fetched abi: %w", err)
	}

	type Response struct {
		status  string
		message string
		Result  interface{} `json:"result"`
	}

	var response Response
	err = json.Unmarshal(abi, &response)
	if err != nil {
		return nil, fmt.Errorf("unmarshaling: %w", err)
	}
	abi = []byte(response.Result.(string))

	return abi, nil
}

func BuildEventModels(abi *eth.ABI) (out []CodegenEvent, err error) {
	names := keys(abi.LogEventsByNameMap)
	sort.StringSlice(names).Sort()

	// We allocate as many names + 16 to potentially account for duplicates
	out = make([]CodegenEvent, 0, len(names)+16)
	for _, name := range names {
		events := abi.FindLogsByName(name)

		for i, event := range events {
			codegenEvent := CodegenEvent{
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

func keys[K comparable, V any](entries map[K]V) (out []K) {
	if len(entries) == 0 {
		return nil
	}

	out = make([]K, len(entries))
	i := 0
	for k := range entries {
		out[i] = k
		i++
	}

	return
}

func (e *CodegenEvent) populateFields(log *eth.LogEventDef) error {
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
