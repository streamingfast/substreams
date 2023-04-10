package templates

type EthereumChain struct {
	ID           string
	DisplayName  string
	ExplorerLink string
}

var EthereumChainsByID = map[string]*EthereumChain{
	"ethereum_mainnet": {
		DisplayName:  "Ethereum Mainnet",
		ExplorerLink: "https://etherscan.io",
	},
}

func init() {
	for k, v := range EthereumChainsByID {
		v.ID = k
	}
}
