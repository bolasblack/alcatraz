# Container Identity Stability Research

## Problem Analysis

### Current Implementation

The current implementation in `internal/runtime/docker.go:207-214` generates container names using SHA256 hash of the absolute path:

```go
func containerName(projectDir string) string {
    absPath, err := filepath.Abs(projectDir)
    if err != nil {
        absPath = projectDir
    }
    hash := sha256.Sum256([]byte(absPath))
    return "alca-" + hex.EncodeToString(hash[:])[:12]
}
```

### Problem

When any of the following occur, the container becomes "orphaned":

1. **Directory moved**: `/home/user/project-a` → `/home/user/projects/project-a`
2. **Directory renamed**: `/home/user/my-app` → `/home/user/my-app-v2`
3. **Alca binary updated**: No effect on container name, but related issue
4. **Symlink resolution changes**: Different absolute path resolved

**Result**: `alca status` shows "Container: Not created" even though the container exists and may be running.

### Impact

- Orphaned containers consuming resources
- User confusion about container state
- Potential data in container volumes becomes inaccessible
- Multiple containers created for same logical project

## Solution Comparison

### Solution 1: Local State File (.alca/state.json)

Store container identity in a local state file within the project directory.

**Implementation**:
```go
// .alca/state.json
{
    "container_id": "abc123def456",
    "container_name": "alca-abc123def456",
    "created_at": "2024-01-15T10:30:00Z",
    "runtime": "docker"
}
```

**Pros**:
- Simple to implement
- Container identity survives directory moves
- Can store additional metadata (creation time, runtime type)
- Follows Terraform state file pattern (industry standard)
- State file moves with project

**Cons**:
- Another file to manage (`.gitignore` needed)
- State file can be deleted/corrupted
- Need migration path for existing containers

**Industry Reference**: Terraform uses local state files (`terraform.tfstate`) for resource tracking. See [Terraform State Management](https://spacelift.io/blog/terraform-state).

---

### Solution 2: Container Labels

Store project identity as Docker/Podman labels on the container itself.

**Implementation**:
```go
// During container creation
docker run --label "alca.project.path=/original/path" \
           --label "alca.project.id=uuid-here" \
           ...

// During status check - find by label
docker ps -a --filter "label=alca.project.id=uuid-here"
```

**Pros**:
- No local files needed
- Labels survive container stop/start
- Industry standard for container metadata
- Can query containers across all projects

**Cons**:
- Cannot survive directory move (unless combined with state file for UUID)
- Label alone doesn't solve the core problem
- Need to generate and store UUID somewhere anyway
- Container must exist to read labels

**Industry Reference**: [Docker Labels Best Practices](https://snyk.io/blog/how-and-when-to-use-docker-labels-oci-container-annotations/).

---

### Solution 3: File Lock/Marker

Use a lock file or marker to claim a container name.

**Implementation**:
```
~/.alca/containers/alca-abc123/project -> /path/to/project
```

**Pros**:
- Central registry of all containers
- Can detect orphans easily

**Cons**:
- Global state management complexity
- Race conditions possible
- Doesn't solve the core identity problem

---

### Solution 4: Hybrid (State File + Labels) - RECOMMENDED

Combine state file for persistent identity with container labels for discovery.

**Implementation**:

1. **First `alca up`**:
   - Generate UUID for project
   - Create `.alca/state.json` with UUID
   - Create container with labels including UUID

2. **Subsequent operations**:
   - Read UUID from state file
   - Find container by label `alca.project.id=<uuid>`

3. **Directory moved**:
   - State file moves with directory
   - Container found by UUID label

```go
// .alca/state.json
{
    "project_id": "550e8400-e29b-41d4-a716-446655440000",
    "container_name": "alca-550e8400",
    "created_at": "2024-01-15T10:30:00Z"
}

// Container labels
--label "alca.project.id=550e8400-e29b-41d4-a716-446655440000"
--label "alca.version=1"
```

**Pros**:
- Survives directory moves
- Survives container recreation
- Can detect and manage orphans
- Clear source of truth (state file)
- Labels enable discovery without state file

**Cons**:
- More complex than single solution
- Need state file migration logic

## Recommendation

### Primary: Hybrid Solution (State File + Labels)

**Rationale**:
1. **Robustness**: Two sources of truth that can validate each other
2. **Flexibility**: Can recover from partial state loss
3. **Industry alignment**: Follows both Terraform (state file) and Docker (labels) patterns
4. **Future-proof**: Enables advanced features like multi-container projects

### Implementation Roadmap

#### Phase 1: State File Foundation
1. Create `internal/state/state.go` package
2. Define state file schema
3. Add state file read/write on `up`/`down`
4. Migration: Adopt existing containers on first `status`

#### Phase 2: Container Labels
1. Add labels to container creation
2. Update status to prefer label-based lookup
3. Fallback to name-based lookup for compatibility

#### Phase 3: Orphan Management
1. `alca list` - Show all alca containers
2. `alca cleanup` - Remove orphaned containers
3. `alca adopt` - Adopt orphan container to current directory

### State File Location Options

| Location | Pros | Cons |
|----------|------|------|
| `.alca/state.json` | Clean, dedicated directory | Extra directory |
| `.alca-state.json` | Single file | Clutters root |
| `.alca.toml` (extend) | Single config file | Mixes config and state |

**Recommendation**: `.alca/state.json` - separates concerns, allows future expansion.

### Migration Strategy

```go
func (d *Docker) Status(ctx context.Context, projectDir string) (ContainerStatus, error) {
    // 1. Try state file first
    state, err := LoadState(projectDir)
    if err == nil && state.ContainerName != "" {
        // Found state, lookup by label
        status, err := d.findByLabel(ctx, state.ProjectID)
        if err == nil {
            return status, nil
        }
    }

    // 2. Fallback: Legacy name-based lookup
    legacyName := legacyContainerName(projectDir)
    status, err := d.inspectContainer(ctx, legacyName)
    if err == nil && status.State != StateNotFound {
        // Found legacy container, adopt it
        d.adoptContainer(projectDir, status)
        return status, nil
    }

    return ContainerStatus{State: StateNotFound}, nil
}
```

## Orphan Container Detection

An orphan container is an alca-managed container whose project no longer exists or has been disconnected.

### Detection Algorithm

```go
func IsOrphanContainer(container ContainerInfo) bool {
    // 1. Get project path from container label
    projectPath := container.Labels["alca.project.path"]
    if projectPath == "" {
        return true // No path label = orphan
    }

    // 2. Check if project directory exists
    if _, err := os.Stat(projectPath); os.IsNotExist(err) {
        return true // Directory doesn't exist = orphan
    }

    // 3. Check if state file exists
    stateFile := filepath.Join(projectPath, ".alca", "state.json")
    if _, err := os.Stat(stateFile); os.IsNotExist(err) {
        return true // No state file = orphan
    }

    // 4. Optionally: verify project ID matches
    state, _ := state.Load(projectPath)
    if state != nil && state.ProjectID != container.Labels["alca.project.id"] {
        return true // Project ID mismatch = orphan
    }

    return false // All checks passed = not orphan
}
```

### Container Labels

| Label | Description | Example |
|-------|-------------|---------|
| `alca.project.id` | Project UUID | `550e8400-e29b-41d4-a716-446655440000` |
| `alca.project.path` | Original project path | `/Users/dev/my-project` |
| `alca.version` | Alca state version | `1` |

### Use Cases

- **alca list**: Show all alca containers with their associated paths and orphan status
- **alca cleanup**: Remove orphaned containers (after confirmation)
- **alca adopt**: Re-associate an orphan container with a project directory

## Appendix: Industry References

1. **Terraform State**: Uses local `terraform.tfstate` for resource tracking
   - [Managing Terraform State](https://spacelift.io/blog/terraform-state)

2. **Docker Labels**: OCI standard for container metadata
   - [Docker Labels Best Practices](https://snyk.io/blog/how-and-when-to-use-docker-labels-oci-container-annotations/)

3. **Docker Compose**: Uses directory name as project identifier (causing orphan issues)
   - [Orphan Containers Issue](https://github.com/docker/compose/issues/9718)

4. **VS Code Dev Containers**: Uses `vsch.quality` label for tracking
   - [Dev Containers Tips](https://code.visualstudio.com/docs/devcontainers/tips-and-tricks)
