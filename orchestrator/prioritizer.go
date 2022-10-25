package orchestrator

//type PrioritizerFunc func([]*Job) []*Job

type WorkPlanPrioritizer interface {
	Prioritize()
}

//NextJob(*WorkPlan) *Job

/*

  XXXXXXx   X   XXXXX     XXXXXX
  X XXxxXXXXXXXXXXXXXXXX XXXXXXX

*/
