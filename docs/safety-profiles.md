# Safety Profiles

Safety profiles let you build a `gog` binary with specific commands removed at compile time. This is designed for AI agent use cases where you want to guarantee certain operations (send, delete, etc.) are physically impossible.

## How It Works

1. Parent command structs (e.g., `GmailCmd`, `DriveCmd`) live in `*_types.go` files with a `//go:build !safety_profile` tag.
2. A code generator (`cmd/gen-safety`) reads these structs via AST parsing, consults a YAML profile, and generates replacement structs containing only the enabled commands. Generated files get `//go:build safety_profile`.
3. `build-safe.sh` runs the generator and compiles with `-tags safety_profile`, producing a binary where disabled commands do not exist.

A standard `go build` (without the tag) ignores all generated files and produces the normal binary. The safety profile system has zero impact on stock builds.

## Building

```bash
./build-safe.sh safety-profiles/agent-safe.yaml
./build-safe.sh safety-profiles/readonly.yaml -o /usr/local/bin/gog-safe
```

## Preset Profiles

- **`full.yaml`** - All commands enabled (useful as a template)
- **`agent-safe.yaml`** - Read, draft, organize. Blocks send, delete, admin.
- **`readonly.yaml`** - Only read/list/search/get. Nothing can be created or modified.

## YAML Format

Each service maps its subcommands to `true` (enabled) or `false` (disabled):

```yaml
gmail:
  search: true
  send: false
  drafts:
    create: true
    send: false
```

Use `service: true` as shorthand to enable all subcommands, or `service: false` to disable the entire service.

## Fail-Closed Semantics

Commands not listed in the YAML are excluded by default. The `--strict` flag (used by `build-safe.sh`) makes this fatal, so new upstream commands never silently appear in safety-profiled builds.

## Utility Commands (Always Included)

A small set of infrastructure commands (`config`, `time`, `version`, `schema`, `agent`, `agent-exit-codes`, `completion`) are always included in every safety-profiled build. These commands cannot access or modify user data, so they bypass YAML filtering. Their keys are tolerated in YAML profiles to avoid "unrecognized key" warnings, but their values have no effect.

The list is defined in the `utilityTypes` map in `cmd/gen-safety/discover.go`. If a new utility command type is added upstream, it must be added there.

## For Contributors: The `*_types.go` Convention

When adding or modifying a parent command struct (one that contains `cmd:""` subcommand fields), edit the corresponding `*_types.go` file, not the original `.go` file.

For example, to add a new Gmail subcommand:

1. Add the field to `internal/cmd/gmail_types.go` (not `gmail.go`)
2. The YAML key is derived from the `name:""` struct tag (not the Go field name)
3. Add the new key to each profile in `safety-profiles/`
4. Run `./build-safe.sh safety-profiles/full.yaml` to verify

The `_types.go` files are the source of truth for command struct definitions. The original `.go` files contain the `Run()` implementations and helper functions.

### Why the Split?

Go build tags require mutual exclusion at the file level. The stock build uses the `_types.go` definitions (with `!safety_profile` tag), and the safety build uses the generated `_cmd_gen.go` replacements (with `safety_profile` tag). Both define the same struct name, so only one can be active per build.
