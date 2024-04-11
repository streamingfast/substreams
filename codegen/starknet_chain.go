package codegen

//go:generate go-enum -f=$GOFILE --marshal --names --nocase

// ENUM(
//
// Mainnet
// Sepolia
// Other
//
// )
type StarknetChain uint
