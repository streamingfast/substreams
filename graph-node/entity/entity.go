package entity

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

func (e *Base) GetID() string {
	return e.ID
}

func (e *Base) SetID(id string) {
	e.ID = id
}

func (e *Base) GetVID() uint64 {
	return e.VID
}

func (e *Base) SetVID(vid uint64) {
	e.VID = vid
}

func (e *Base) SetBlockRange(br *BlockRange) {
	e.BlockRange = br
}

func (e *Base) GetBlockRange() *BlockRange {
	return e.BlockRange
}

func (e *Base) Exists() bool {
	return e.exists
}

func (e *Base) SetExists(exists bool) {
	e.exists = exists
}

func (e *Base) SetMutated(step int) {
	e.MutatedOnStep = step
}

func (e *Base) SetUpdatedBlockNum(blockNum uint64) {
	e.UpdatedBlockNum = blockNum
}
