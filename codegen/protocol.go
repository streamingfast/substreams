package codegen

//go:generate go-enum -f=$GOFILE --marshal --names --nocase

// ENUM(
//
//	Ethereum
//	Starknet
//	Other
//
// )
type Protocol uint
