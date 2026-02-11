package sync

// Mutagen JSON types for parsing `mutagen sync list --template='{{json .}}'` output.
//
// Schema (mutagen v0.18.1, via ExportSessions):
//
//	[{
//	  "identifier": "sync_...",           // session identifier
//	  "version": 1,                       // session version
//	  "creationTime": "2026-...Z",        // RFC3339
//	  "creatingVersion": "0.18.1",        // mutagen version that created it
//	  "alpha": {                           // local endpoint
//	    "protocol": "local",
//	    "path": "/Users/.../project",
//	    "connected": true,
//	    "scanned": true,
//	    "directories": 401,
//	    "files": 2493,
//	    "totalFileSize": 15740865,
//	    "ignore": {}, "symlink": {}, "watch": {}, "permissions": {}, "compression": {}
//	  },
//	  "beta": {                            // container endpoint
//	    "protocol": "docker",
//	    "host": "<container-id>",
//	    "path": "/workspace",
//	    ...                                // same structure as alpha
//	  },
//	  "name": "alca-<projectID>-0",       // session name
//	  "paused": false,
//	  "status": "watching",               // "connecting", "watching", "scanning", "reconciling", etc.
//	  "successfulCycles": 724,
//	  "ignore": {"paths": [".alca.cache", "out"]},
//	  "symlink": {}, "watch": {}, "permissions": {}, "compression": {},
//	  "conflicts": [{                      // only present when conflicts exist
//	    "root": "test-conflict.txt",       // conflict root, relative to sync root
//	    "alphaChanges": [{
//	      "path": "test-conflict.txt",     // change path, relative to sync root (NOT to root)
//	      "old": null,                     // null when file didn't exist before
//	      "new": {"kind": "file", "digest": "e6f0f9..."}
//	    }],
//	    "betaChanges": [{
//	      "path": "test-conflict.txt",
//	      "old": null,
//	      "new": {"kind": "file", "digest": "415adb..."}
//	    }]
//	  }]
//	}]
//
// We only parse the fields we need (conflicts). Unknown fields are ignored by
// encoding/json.Unmarshal.

type mutagenSession struct {
	Conflicts []mutagenConflict `json:"conflicts,omitempty"`
}

type mutagenConflict struct {
	Root         string          `json:"root"`
	AlphaChanges []mutagenChange `json:"alphaChanges"`
	BetaChanges  []mutagenChange `json:"betaChanges"`
}

type mutagenChange struct {
	Path string        `json:"path"`
	Old  *mutagenEntry `json:"old"`
	New  *mutagenEntry `json:"new"`
}

type mutagenEntry struct {
	Kind   string `json:"kind"`
	Digest string `json:"digest,omitempty"`
}

// Entry kind constants matching mutagen's exported JSON string values.
const (
	entryKindNothing   = "" // nil entry (does not exist)
	entryKindFile      = "file"
	entryKindDirectory = "directory"
	entryKindSymlink   = "symlink"
)
