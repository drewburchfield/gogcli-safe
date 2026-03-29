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

A small set of infrastructure commands (`config`, `time`, `version`, `schema`, `agent`, `agent-exit-codes`, `completion`) are always included in every safety-profiled build. These commands manage local CLI configuration and metadata rather than Google Workspace data, so they bypass YAML filtering. Note that `config set` can modify local settings; if this is a concern for your threat model, restrict filesystem write access at the agent framework level. Their keys are tolerated in YAML profiles to avoid "unrecognized key" warnings, but their values have no effect.

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

## Maintenance: `--verify`, `--sync`, and `--update-profiles`

Three modes help keep types files and YAML profiles in sync as commands evolve:

### `--verify` (CI integration)

```bash
go run ./cmd/gen-safety --verify
```

Checks that every CLI-level service type has a corresponding `*_types.go` file and that no struct is duplicated across types and source files. Exits non-zero if problems are found. Intended for CI pipelines to catch drift automatically.

### `--sync` (migration tool)

```bash
go run ./cmd/gen-safety --sync
```

When a new top-level service is added (e.g., `AdminCmd` in `admin.go`), `--sync` detects it and generates the corresponding `admin_types.go` file with the `//go:build !safety_profile` tag. If a types file already exists, `--sync` skips it to avoid overwriting manual edits. After running `--sync`, remove the struct definition from the original source file and add the new command keys to each YAML profile.

### `--update-profiles` (YAML profile maintenance)

```bash
go run ./cmd/gen-safety --update-profiles            # add missing keys
go run ./cmd/gen-safety --update-profiles --dry-run  # preview without writing
```

Scans all YAML profiles in `safety-profiles/` (plus `safety-profile.example.yaml` if present) for missing command keys and adds them with sensible defaults: `true` for `full.yaml`, `false` for all others. Preserves existing formatting, comments, and indentation. New keys are tagged with `# NEW` so they are easy to find and review. Bool shorthand sections (e.g., `classroom: false`) are left untouched.

### Typical workflow after merging upstream

1. `git merge upstream/main` and resolve conflicts (accept upstream for source files; review `*_types.go` conflicts manually since those are the safety profile source of truth)
2. `go run ./cmd/gen-safety --verify` to see what needs attention
3. For each DUPLICATE: remove the struct definition from the source file (the types file is the source of truth; update it manually if upstream added new fields)
4. For each MISSING: `go run ./cmd/gen-safety --sync` to generate the types file, then remove the struct from the source file
5. `go build ./cmd/gog/` to verify stock build
6. `go run ./cmd/gen-safety --update-profiles` to add missing YAML keys (defaults: `true` for full.yaml, `false` for others). Use `--dry-run` to preview.
7. Review the added keys and adjust defaults for agent-safe.yaml as needed
8. `./build-safe.sh safety-profiles/full.yaml` to verify safety build
