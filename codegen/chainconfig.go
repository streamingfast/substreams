package codegen

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
		FirehoseEndpoint: "mainnet.eth.streamingfast.io:443",
		Network:          "mainnet",
		SupportsCalls:    true,
	},
	"bnb": {
		DisplayName:      "BNB",
		ExplorerLink:     "https://bscscan.com",
		FirehoseEndpoint: "bnb.streamingfast.io:443",
		Network:          "bsc",
		SupportsCalls:    true,
	},
	"polygon": {
		DisplayName:      "Polygon",
		ExplorerLink:     "https://polygonscan.com",
		FirehoseEndpoint: "polygon.streamingfast.io:443",
		Network:          "polygon",
		SupportsCalls:    true,
	},
	"amoy": {
		DisplayName:      "Polygon Amoy Testnet",
		ExplorerLink:     "https://www.okx.com/web3/explorer/amoy",
		FirehoseEndpoint: "amoy.substreams.pinax.network:443",
		Network:          "amoy",
		SupportsCalls:    true,
	},
	"arbitrum": {
		DisplayName:      "Arbitrum",
		ExplorerLink:     "https://arbiscan.io",
		FirehoseEndpoint: "arb-one.streamingfast.io:443",
		Network:          "arbitrum",
		SupportsCalls:    true,
	},
	"holesky": {
		DisplayName:      "Holesky",
		ExplorerLink:     "https://holesky.etherscan.io/",
		FirehoseEndpoint: "holesky.eth.streamingfast.io:443",
		Network:          "holesky",
		SupportsCalls:    true,
	},
	"sepolia": {
		DisplayName:      "Sepolia Testnet",
		ExplorerLink:     "https://sepolia.etherscan.io",
		FirehoseEndpoint: "sepolia.streamingfast.io:443",
		Network:          "sepolia",
		SupportsCalls:    true,
	},
	"optimism": {
		DisplayName:      "Optimism Mainnet",
		ExplorerLink:     "https://optimistic.etherscan.io",
		FirehoseEndpoint: "opt-mainnet.streamingfast.io:443",
		Network:          "optimism",
		SupportsCalls:    false,
	},
	"avalanche": {
		DisplayName:      "Avalanche C-chain",
		ExplorerLink:     "https://subnets.avax.network/c-chain",
		FirehoseEndpoint: "avalanche-mainnet.streamingfast.io:443",
		Network:          "avalanche",
		SupportsCalls:    false,
	},
	"chapel": {
		DisplayName:      "BNB Chapel Testnet",
		ExplorerLink:     "https://testnet.bscscan.com/",
		FirehoseEndpoint: "chapel.substreams.pinax.network:443",
		Network:          "chapel",
		SupportsCalls:    true,
	},
	"injective-mainnet": {
		DisplayName:      "Injective Mainnet",
		ExplorerLink:     "https://explorer.injective.network/",
		FirehoseEndpoint: "mainnet.injective.streamingfast.io:443",
		Network:          "injective-mainnet",
	},
	"injective-testnet": {
		DisplayName:      "Injective Testnet",
		ExplorerLink:     "https://testnet.explorer.injective.network/",
		FirehoseEndpoint: "testnet.injective.streamingfast.io:443",
		Network:          "injective-testnet",
	},
	"starknet-mainnet": {
		DisplayName:      "Starknet Mainnet Transactions",
		ExplorerLink:     "https://starkscan.co/",
		FirehoseEndpoint: "mainnet.starknet.streamingfast.io:443",
		Network:          "starknet-mainnet",
	},
	"starknet-testnet": {
		DisplayName:      "Starknet Testnet Transactions",
		ExplorerLink:     "",
		FirehoseEndpoint: "testnet.starknet.streamingfast.io:443",
		Network:          "starknet-testnet",
	},
	"solana-mainnet-beta": {
		DisplayName:      "Solana Mainnet",
		Network:          "solana-mainnet-beta",
		FirehoseEndpoint: "mainnet.solana.streamingfast.io:443",
	},
	"vara-mainnet": {
		DisplayName:      "Vara Mainnet",
		ExplorerLink:     "https://vara.subscan.io/",
		FirehoseEndpoint: "mainnet.vara.streamingfast.io:443",
		Network:          "vara-mainnet",
	},
	"vara-testnet": {
		DisplayName:      "Vara Testnet",
		ExplorerLink:     "",
		FirehoseEndpoint: "testnet.vara.streamingfast.io:443",
		Network:          "vara-testnet",
	},
}
