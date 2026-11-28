package rows

//go:generate go-enum -f=$GOFILE --marshal --names

// Op is the operation a row represents. The schema producer only emits [OpCreate].
//
// ENUM(
//
//	create = 1
//	update
//	delete
//
// )
type Op uint8
