package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// DetectConflicts detects sync conflicts for a given mutagen session.
func (e *SyncEnv) DetectConflicts(ctx context.Context, sessionName string) ([]ConflictInfo, error) {
	output, err := e.Sessions.ListSessionJSON(ctx, sessionName)
	if err != nil {
		return nil, err
	}

	var sessions []mutagenSession
	if err := json.Unmarshal(output, &sessions); err != nil {
		return nil, fmt.Errorf("failed to parse session JSON: %w", err)
	}

	now := time.Now()
	var conflicts []ConflictInfo
	for _, sess := range sessions {
		for _, c := range sess.Conflicts {
			infos := conflictToInfos(c, now)
			conflicts = append(conflicts, infos...)
		}
	}
	return conflicts, nil
}

func conflictToInfos(c mutagenConflict, now time.Time) []ConflictInfo {
	alphaStates := buildChangeStates(c.Root, c.AlphaChanges)
	betaStates := buildChangeStates(c.Root, c.BetaChanges)

	// Collect all unique paths.
	paths := make(map[string]struct{}, len(alphaStates)+len(betaStates))
	for p := range alphaStates {
		paths[p] = struct{}{}
	}
	for p := range betaStates {
		paths[p] = struct{}{}
	}

	var infos []ConflictInfo
	for p := range paths {
		infos = append(infos, ConflictInfo{
			Path:           p,
			LocalState:     alphaStates[p],
			ContainerState: betaStates[p],
			DetectedAt:     now,
		})
	}
	return infos
}

// buildChangeStates maps each change to its resolved path and state string.
// In mutagen's exported JSON, Change.Path is relative to the sync root (not to
// Conflict.Root). When Path is empty, the change applies to Root itself.
func buildChangeStates(root string, changes []mutagenChange) map[string]string {
	states := make(map[string]string, len(changes))
	for _, ch := range changes {
		path := ch.Path
		if path == "" {
			path = root
		}
		states[path] = changeState(ch)
	}
	return states
}

func changeState(ch mutagenChange) string {
	oldKind := entryKindNothing
	if ch.Old != nil {
		oldKind = ch.Old.Kind
	}
	newKind := entryKindNothing
	if ch.New != nil {
		newKind = ch.New.Kind
	}

	if newKind == entryKindDirectory {
		return "directory"
	}
	if oldKind == entryKindNothing && newKind != entryKindNothing {
		return "created"
	}
	if oldKind != entryKindNothing && newKind == entryKindNothing {
		return "deleted"
	}
	if oldKind != entryKindNothing && newKind != entryKindNothing {
		return "modified"
	}
	return "unknown"
}
