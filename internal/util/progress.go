// Package util provides shared utility functions across CLI and runtime.
package util

import (
	"fmt"
	"io"
)

// Progress writes a progress message if not in quiet mode.
func Progress(w io.Writer, format string, args ...any) {
	if w != nil {
		_, _ = fmt.Fprintf(w, format, args...)
	}
}

// ProgressStep writes a progress message with → prefix (step in progress).
func ProgressStep(w io.Writer, format string, args ...any) {
	Progress(w, "→ "+format, args...)
}

// ProgressDone writes a progress message with ✓ prefix (step completed).
func ProgressDone(w io.Writer, format string, args ...any) {
	Progress(w, "✓ "+format, args...)
}
