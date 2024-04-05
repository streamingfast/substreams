package pbssinternal

import "encoding/json"

func (x *ProcessRangeRequest) WasmConfig(module string) string {
	return x.WasmModules[module]
}

type RPCCallWasmModuleConfiguration struct {
	Endpoint string `json:"endpoint"`
}

func GetRPCCallWasmModuleConfiguration(r *ProcessRangeRequest) (*RPCCallWasmModuleConfiguration, error) {
	configurationString := r.WasmConfig("WASM_MODULE_TYPE_RPC_CALL")

	var config *RPCCallWasmModuleConfiguration
	err := json.Unmarshal([]byte(configurationString), &config)
	if err != nil {
		return nil, err
	}

	return config, nil
}

func ToWasmModuleTypeArray(s []string) []WASMModuleType {
	var result []WASMModuleType
	for _, v := range s {
		result = append(result, WASMModuleType(WASMModuleType_value[v]))
	}
	return result
}
