package exec

import "bytes"

type ErrorExecutor struct {
	message    string
	stackTrace []string
}

const maxErrorSize = 18000 // Some load balancer will fail close to 20k

func (e *ErrorExecutor) Error() string {
	if len(e.stackTrace) == 0 {
		return e.message
	}

	b := bytes.NewBuffer(nil)
	// stack trace section will also contain the logs of the execution
	for _, stackTraceLine := range e.stackTrace {
		b.WriteString(stackTraceLine)
		b.WriteString("\n")
	}
	traces := b.String()

	out := e.message + "\n\n----- stack trace / logs -----\n"
	if length := len(traces); length > maxErrorSize {
		out += "[TRUNCATED]\n" + traces[length-maxErrorSize:length]
	} else {
		out += traces
	}

	return out
}
