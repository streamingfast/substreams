package orchestrator

type Config struct {
	parallelSubrequests       int
	blockRangeSizeSubrequests int
}

func NewConfig(parallelSubrequests int, blockRangeSizeSubrequests int) *Config {
	return &Config{
		parallelSubrequests:       parallelSubrequests,
		blockRangeSizeSubrequests: blockRangeSizeSubrequests,
	}
}

func (c *Config) GetParallelSubrequests() int {
	return c.parallelSubrequests
}

func (c *Config) GetBlockRangeSizeSubrequests() int {
	return c.blockRangeSizeSubrequests
}
