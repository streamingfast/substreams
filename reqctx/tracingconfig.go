package reqctx

type TracingConf struct {
	ModuleExecution bool
}

func NewTracingConf(
	moduleExecution bool,
) *TracingConf {
	return &TracingConf{
		ModuleExecution: moduleExecution,
	}
}
