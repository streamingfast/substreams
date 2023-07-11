package codegen

//go:generate go-enum -f=$GOFILE --marshal --names --nocase

// ENUM(
//
// Mainnet
// BNB
// Polygon
// Goerli
// // Sepolia
// Other
//
// )
type EthereumChain uint
