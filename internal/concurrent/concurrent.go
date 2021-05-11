package concurrent

import "context"

// Runnable is a task executed (generally) in a separate thread.
type Runnable interface {

	// Name returns the task name.
	Name() string

	// Run executes the task.
	Run(context.Context) error
}
