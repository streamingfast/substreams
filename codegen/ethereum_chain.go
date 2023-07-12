package codegen

//go:generate go-enum -f=$GOFILE --marshal --names --nocase

// ENUM(
//
// Mainnet
// BNB
// Polygon
// Goerli
// Mumbai
// // Sepolia
// Other
//
// )
type EthereumChain uint
