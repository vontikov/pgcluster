package logging

import (
	"io"
	"os"

	log "github.com/hashicorp/go-hclog"
)

var level string = "INFO"
var output io.Writer = os.Stdout

// Logger defines the interface that is used for logging operations.
type Logger log.Logger

// SetLevel sets logging level.
func SetLevel(v string) {
	level = v
}

// Level returns logging level.
func Level() string {
	return level
}

// Output sets logging output.
func Output(v io.Writer) {
	output = v
}

// NewLogger creates a new Logger instance with name n.
func NewLogger(n string) Logger {
	return log.New(&log.LoggerOptions{
		Name:   n,
		Level:  log.LevelFromString(level),
		Output: output,
		Color:  log.ForceColor,
	})
}
