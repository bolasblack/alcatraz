package cli

import "errors"

// Sentinel errors for the cli package.
var (
	// errSkipFirewall signals that firewall setup should be skipped without reporting an error.
	errSkipFirewall = errors.New("skip firewall")
	// errSyncConflicts is returned when unresolved sync conflicts block an operation.
	errSyncConflicts = errors.New("sync conflicts")
	// errProjectPathMismatch is returned when the project directory has moved since the container was created.
	errProjectPathMismatch = errors.New("project path mismatch")
)
