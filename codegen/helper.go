package codegen

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/iancoleman/strcase"
	"github.com/streamingfast/dhttp"
	"github.com/streamingfast/eth-go"
	"golang.org/x/net/context"
)

var httpClient = http.Client{
	Transport: dhttp.NewLoggingRoundTripper(zlog, tracer, http.DefaultTransport),
	Timeout:   30 * time.Second,
}

func init() {
	// At least we define a sane defa
	http.DefaultClient.Timeout = 30 * time.Second
}

type CodegenEvent struct {
	RustName string
	Fields   map[int]string
	Values   []string
}

type ProtoEvent struct {
	EventIndex     int            // >=1
	EventName      string         //Transfer
	LowerAndPlural string         //transfers
	Fields         map[int]string //{[1]"to", [2]"from"}
	IndexesPlus    []int          //[6, 7]
}

type RustEvent struct {
	RustName       string         // Approval
	LowerAndPlural string         // "approvalsforalls"
	Fields         map[int]string // {[1]"to", [2]"from"}
	FieldValues    []string       // {"", "Hex(&blk.hash).to_string()
}

func (*CodegenEvent) getProtoEvent(eventIndex int, event *CodegenEvent) ProtoEvent {
	protoEvent := &ProtoEvent{
		EventIndex:     eventIndex,
		EventName:      event.RustName,
		LowerAndPlural: fmt.Sprintf("%ss", strings.ToLower(event.RustName)),
		Fields:         event.Fields,
		IndexesPlus:    []int{0},
	}
	for i := range event.Fields {
		protoEvent.IndexesPlus = append(protoEvent.IndexesPlus, i+5)
	}
	return *protoEvent
}

func (*CodegenEvent) getRustEvent(event *CodegenEvent) RustEvent {
	rustEvent := &RustEvent{
		RustName:       event.RustName,
		LowerAndPlural: fmt.Sprintf("%ss", strings.ToLower(event.RustName)),
		Fields:         event.Fields,
		FieldValues:    []string{""},
	}

	for _, value := range event.Values {
		if strings.HasSuffix(value, ".to_string()") {
			rustEvent.FieldValues = append(rustEvent.FieldValues, value)
		} else {
			rustEvent.FieldValues = append(rustEvent.FieldValues, fmt.Sprintf("%s.to_string()", value))
		}
	}
	return *rustEvent
}

func GetContractABI(ctx context.Context, contract eth.Address) (string, *eth.ABI, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("https://api.etherscan.io/api?module=contract&action=getabi&address=%s&apikey=YourApiKeyToken", contract.Pretty()), nil)
	if err != nil {
		return "", nil, fmt.Errorf("new request: %w", err)
	}

	res, err := httpClient.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("getting contract abi from etherscan: %w", err)
	}
	defer res.Body.Close()

	type Response struct {
		Result interface{} `json:"result"`
	}

	var response Response
	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		return "", nil, fmt.Errorf("unmarshaling: %w", err)
	}

	abiContent, ok := response.Result.(string)
	if !ok {
		return "", nil, fmt.Errorf(`invalid response "Result" field type, expected "string" got "%T"`, response.Result)
	}

	ethABI, err := eth.ParseABIFromBytes([]byte(abiContent))
	if err != nil {
		return "", nil, fmt.Errorf("parsing abi: %w", err)
	}

	return abiContent, ethABI, nil
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
			i++
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

	e.Fields = map[int]string{}
	for i, parameter := range log.Parameters {
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

		e.Fields[i+1] = name
		e.Values = append(e.Values, toJsonCode)
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
