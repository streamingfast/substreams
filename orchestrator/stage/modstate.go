package stage

type ModuleState struct {
	name string

	completedSegments int
	scheduledSegments map[int]bool

	readyUpToBlock     uint64
	mergedUpToBlock    uint64
	scheduledUpToBlock uint64
}
