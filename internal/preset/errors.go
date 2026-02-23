package preset

import "errors"

// Sentinel errors for the preset package.
var (
	// ErrNoPresetFiles is returned when no .alca.*.toml files are found in the target directory.
	ErrNoPresetFiles = errors.New("no preset files found")

	// ErrInvalidPresetURL is returned when a preset URL is malformed.
	ErrInvalidPresetURL = errors.New("invalid preset URL")

	// ErrInvalidSourceComment is returned when a source comment is malformed.
	ErrInvalidSourceComment = errors.New("invalid source comment")

	// ErrNoSourceComment is returned when no source comment is found in a file.
	ErrNoSourceComment = errors.New("no source comment found")
)
