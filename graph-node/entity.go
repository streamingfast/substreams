package graphnode

type Base struct {
	ID              string      `db:"id" csv:"id"`         // text key
	VID             uint64      `db:"vid" csv:"-" poi:"-"` // version
	BlockRange      *BlockRange `db:"block_range" csv:"block_range"`
	UpdatedBlockNum uint64      `db:"_updated_block_number" csv:"updated_block_number" poi:"-"`
	exists          bool

	MutatedOnStep int `db:"-" csv:"-" poi:"-"`
}

func NewBase(id string) Base {
	return Base{ID: id}
}

func (b *Base) Default() {}

func (b *Base) GetID() string {
	return b.ID
}

func (b *Base) SetID(id string) {
	b.ID = id
}

func (b *Base) GetVID() uint64 {
	return b.VID
}

func (b *Base) SetVID(vid uint64) {
	b.VID = vid
}

func (b *Base) SetBlockRange(br *BlockRange) {
	b.BlockRange = br
}

func (b *Base) GetBlockRange() *BlockRange {
	return b.BlockRange
}

func (b *Base) Exists() bool {
	return b.exists
}

func (b *Base) SetExists(exists bool) {
	b.exists = exists
}

func (b *Base) SetMutated(step int) {
	b.MutatedOnStep = step
}

func (b *Base) SetUpdatedBlockNum(blockNum uint64) {
	b.UpdatedBlockNum = blockNum
}
