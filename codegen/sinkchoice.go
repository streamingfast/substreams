package codegen

//go:generate go-enum -f=$GOFILE --marshal --names --nocase

// ENUM(
//
//		No
//		Db
//	 Graph
//
// )
type SinkChoice int
