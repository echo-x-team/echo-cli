package execution

import "fmt"

// stageError wraps an underlying error with a stable stage identifier so
// the run_task completion summary can provide better failure analysis.
type stageError struct {
	Stage string
	Err   error
}

func (e stageError) Error() string {
	if e.Stage == "" {
		return fmt.Sprintf("%v", e.Err)
	}
	return fmt.Sprintf("%s: %v", e.Stage, e.Err)
}

func (e stageError) Unwrap() error { return e.Err }
