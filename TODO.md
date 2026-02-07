# TODO

## Mutagen Sync Conflict Handling

When mutagen sync is enabled, file conflicts occur frequently. Need to:

1. **Conflict detection and user notification** — detect when mutagen reports conflicts and surface a clear warning to the user (e.g., during `alca status` or as part of `alca up` output)
2. **Conflict resolution commands** — provide `alca` subcommands to help users inspect and resolve mutagen sync conflicts (e.g., list conflicts, accept local/remote, reset sync state)
