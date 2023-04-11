package codegen

//go:generate go-enum -f=$GOFILE --marshal --names --nocase

// ENUM(
//
//	Ethereum
//	Other
//
// )
type Protocol uint
