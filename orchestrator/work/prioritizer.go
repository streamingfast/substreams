package work

type Prioritizer interface {
	Sort(jobs []*Job) []*Job
}
