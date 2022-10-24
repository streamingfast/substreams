package exec

import "bytes"

type ErrorExecutor struct {
	message    string
	stackTrace []string
}

func (e *ErrorExecutor) Error() string {
	b := bytes.NewBuffer(nil)

	b.WriteString(e.message)

	if len(e.stackTrace) > 0 {
		// stack trace section will also contain the logs of the execution
		b.WriteString("\n----- stack trace -----\n")
		for _, stackTraceLine := range e.stackTrace {
			b.WriteString(stackTraceLine)
			b.WriteString("\n")
		}
	}

	return b.String()
}
