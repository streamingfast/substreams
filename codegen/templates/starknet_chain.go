package templates

type StarknetChain struct {
	ID          string
	DisplayName string
	Network     string
}

var StarknetChainsByID = map[string]*StarknetChain{
	"Mainnet": {
		DisplayName: "Starknet Mainnet",
		Network:     "starknet-mainnet",
	},
	"Sepolia": {
		DisplayName: "Starknet Sepolia",
		Network:     "starknet-sepolia",
	},
}

func init() {
	for k, v := range StarknetChainsByID {
		v.ID = k
	}
}
