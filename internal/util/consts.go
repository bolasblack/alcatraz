package util

import "fmt"

// Application-level directory paths relative to user home.
const (
	AlcatrazDir = ".alcatraz"
	FilesDir    = ".alcatraz/files"
)

// MutagenSessionPrefix returns the session name prefix for a project.
// All mutagen sessions for a project share this prefix.
func MutagenSessionPrefix(projectID string) string {
	return fmt.Sprintf("alca-%s-", projectID)
}

// MutagenSessionName generates a unique session name for a project mount.
// Format: alca-<projectID>-<mountIndex>
func MutagenSessionName(projectID string, mountIndex int) string {
	return fmt.Sprintf("alca-%s-%d", projectID, mountIndex)
}
