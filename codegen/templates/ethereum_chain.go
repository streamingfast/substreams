package templates

type EthereumChain struct {
	ID                     string
	DisplayName            string
	ExplorerLink           string
	ApiEndpoint            string
	DefaultContractAddress string
	DefaultContractName    string
	FirehoseEndpoint       string
	Network                string
}

var EthereumChainsByID = map[string]*EthereumChain{
	"Mainnet": {
		DisplayName:            "Ethereum Mainnet",
		ExplorerLink:           "https://etherscan.io",
		ApiEndpoint:            "https://api.etherscan.io",
		DefaultContractAddress: "bc4ca0eda7647a8ab7c2061c2e118a18a936f13d",
		DefaultContractName:    "Bored Ape Yacht Club",
		FirehoseEndpoint:       "mainnet.eth.streamingfast.io:443",
		Network:                "mainnet",
	},
	"BNB": {
		DisplayName:            "BNB",
		ExplorerLink:           "https://bscscan.com",
		ApiEndpoint:            "https://api.bscscan.com",
		DefaultContractAddress: "0x0e09fabb73bd3ade0a17ecc321fd13a19e81ce82",
		DefaultContractName:    "CAKE Token",
		FirehoseEndpoint:       "bnb.streamingfast.io:443",
		Network:                "bsc",
	},
	"Polygon": {
		DisplayName:            "Polygon",
		ExplorerLink:           "https://polygonscan.com",
		ApiEndpoint:            "https://api.polygonscan.com",
		DefaultContractAddress: "0x7ceb23fd6bc0add59e62ac25578270cff1b9f619",
		DefaultContractName:    "WETH Token",
		FirehoseEndpoint:       "polygon.streamingfast.io:443",
		Network:                "polygon",
	},
	"Arbitrum": {
		DisplayName:            "Arbitrum",
		ExplorerLink:           "https://arbiscan.io",
		ApiEndpoint:            "https://api.arbiscan.io",
		DefaultContractAddress: "0x82aF49447D8a07e3bd95BD0d56f35241523fBab1",
		DefaultContractName:    "WETH Token",
		FirehoseEndpoint:       "arb-one.streamingfast.io:443",
		Network:                "arbitrum",
	},
	"Goerli": {
		DisplayName:            "Goerli Testnet",
		ExplorerLink:           "https://goerli.etherscan.io",
		ApiEndpoint:            "https://api-goerli.etherscan.io",
		DefaultContractAddress: "0x4f7a67464b5976d7547c860109e4432d50afb38e",
		DefaultContractName:    "GETH Token",
		FirehoseEndpoint:       "goerli.eth.streamingfast.io:443",
		Network:                "goerli",
	},
	"Mumbai": {
		DisplayName:            "Mumbai Testnet",
		ExplorerLink:           "https://mumbai.polygonscan.com",
		ApiEndpoint:            "https://api-mumbai.polygonscan.com",
		DefaultContractAddress: "0xFCe7187B24FCDc9feFfE428Ec9977240C6F7006D",
		DefaultContractName:    "USDT Token",
		FirehoseEndpoint:       "mumbai.streamingfast.io:443",
		Network:                "mumbai",
	},
	// "Sepolia": {
	// 	DisplayName:            "Sepolia Testnet",
	// 	ExplorerLink:           "https://sepolia.etherscan.io",
	// 	ApiEndpoint:            "https://api-sepolia.etherscan.io",
	// 	DefaultContractAddress: "0x1f9840a85d5aF5bf1D1762F925BDADdC4201F984",
	// 	DefaultContractName:    "UNI Token",
	// 	FirehoseEndpoint:       "sepolia.streamingfast.io:443",
	// },
}

func init() {
	for k, v := range EthereumChainsByID {
		v.ID = k
	}
}
