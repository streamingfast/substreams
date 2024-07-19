package subgraph

type ChainConfig struct {
	ID               string // Public
	DisplayName      string // Public
	ExplorerLink     string
	ApiEndpoint      string
	FirehoseEndpoint string
	Network          string
	SupportsCalls    bool
}

var ChainConfigByID = map[string]*ChainConfig{
	"mainnet": {
		DisplayName:      "Ethereum Mainnet",
		ExplorerLink:     "https://etherscan.io",
		ApiEndpoint:      "https://api.etherscan.io",
		FirehoseEndpoint: "mainnet.eth.streamingfast.io:443",
		Network:          "mainnet",
		SupportsCalls:    true,
	},
	"bnb": {
		DisplayName:      "BNB",
		ExplorerLink:     "https://bscscan.com",
		ApiEndpoint:      "https://api.bscscan.com",
		FirehoseEndpoint: "bnb.streamingfast.io:443",
		Network:          "bsc",
		SupportsCalls:    true,
	},
	"polygon": {
		DisplayName:      "Polygon",
		ExplorerLink:     "https://polygonscan.com",
		ApiEndpoint:      "https://api.polygonscan.com",
		FirehoseEndpoint: "polygon.streamingfast.io:443",
		Network:          "polygon",
		SupportsCalls:    true,
	},
	"amoy": {
		DisplayName:      "Polygon Amoy Testnet",
		ExplorerLink:     "https://www.okx.com/web3/explorer/amoy",
		ApiEndpoint:      "",
		FirehoseEndpoint: "amoy.substreams.pinax.network:443",
		Network:          "amoy",
		SupportsCalls:    true,
	},
	"arbitrum": {
		DisplayName:      "Arbitrum",
		ExplorerLink:     "https://arbiscan.io",
		ApiEndpoint:      "https://api.arbiscan.io",
		FirehoseEndpoint: "arb-one.streamingfast.io:443",
		Network:          "arbitrum",
		SupportsCalls:    true,
	},
	"holesky": {
		DisplayName:      "Holesky",
		ExplorerLink:     "https://holesky.etherscan.io/",
		ApiEndpoint:      "https://api-holesky.etherscan.io",
		FirehoseEndpoint: "holesky.eth.streamingfast.io:443",
		Network:          "holesky",
		SupportsCalls:    true,
	},
	"sepolia": {
		DisplayName:      "Sepolia Testnet",
		ExplorerLink:     "https://sepolia.etherscan.io",
		ApiEndpoint:      "https://api-sepolia.etherscan.io",
		FirehoseEndpoint: "sepolia.streamingfast.io:443",
		Network:          "sepolia",
		SupportsCalls:    true,
	},
	"optimism": {
		DisplayName:      "Optimism Mainnet",
		ExplorerLink:     "https://optimistic.etherscan.io",
		ApiEndpoint:      "https://api-optimistic.etherscan.io",
		FirehoseEndpoint: "opt-mainnet.streamingfast.io:443",
		Network:          "optimism",
		SupportsCalls:    false,
	},
	"avalanche": {
		DisplayName:      "Avalanche C-chain",
		ExplorerLink:     "https://subnets.avax.network/c-chain",
		ApiEndpoint:      "",
		FirehoseEndpoint: "avalanche-mainnet.streamingfast.io:443",
		Network:          "avalanche",
		SupportsCalls:    false,
	},
	"chapel": {
		DisplayName:      "BNB Chapel Testnet",
		ExplorerLink:     "https://testnet.bscscan.com/",
		ApiEndpoint:      "",
		FirehoseEndpoint: "chapel.substreams.pinax.network:443",
		Network:          "chapel",
		SupportsCalls:    true,
	},
}
