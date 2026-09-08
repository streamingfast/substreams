package sql

type Context struct {
	blockNumber int
}

func NewContext() *Context {
	return &Context{}
}

func (c *Context) SetNumber(id int) {
	c.blockNumber = id
}

func (c *Context) BlockNumber() int {
	return c.blockNumber
}
