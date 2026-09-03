# Commands and flags

The `vellum` binary is a thin adapter over the library facade
([`vellum.go`](../library/facade.md)): every verb below parses its flags,
calls exactly one facade method, and writes the result. It contains no
business logic of its own.

Every command accepts `--json`, which writes a `descriptor.Envelope` to
stdout instead of the human-readable default — see [the output
envelope](../contract/envelope.md) — with one documented exception: `schema`
always writes raw, unwrapped JSON Schema, because the payload it returns
already carries its own `$schema` and `$id`.

Errors always land in the envelope's `errors` array on stdout in `--json`
mode, exactly as a success does; in the default human mode they print to
stderr. Every command's own exit code follows one convention:

| Code | Meaning |
|---|---|
| `0` | Success. |
| `1` | The command was well-formed and the operation it asked for failed — a rejected specification, an unreconciled binding. |
| `2` | The command's own invocation was malformed — a bad flag, a missing argument, a file that does not exist. Nothing was attempted. |

## Command index

| Command | Summary |
|---|---|
| `vellum compose [spec-file] --format <fmt> [-o <path>] [--json]` | Render a specification (JSON or YAML, from a file or stdin) to `--format`. Writes to `-o`, or to stdout when `-o` is omitted and `--json` was not requested — the two cannot share one stdout stream. |
| `vellum validate [spec-file] --format <fmt> [--json]` | Check a specification against `--format`'s capability matrix. Never writes an artifact. |
| `vellum fill --template <path> --binding <path> [--data <path> \| --data-json <json>] [-o <path>] [--json]` | Bind data into an OOXML template according to a binding document. Data defaults to stdin when neither `--data` nor `--data-json` is given. |
| `vellum inspect [template-file] [--json]` | Report a template's anchor inventory and font requirements, without a binding. |
| `vellum boxes --format <fmt> [--theme <id>] [--json]` | Report the asset slots a theme offers a format. Answerable before any specification exists. |
| `vellum capabilities --format <fmt> [--json]` | Report the declared (feature, format) outcome matrix. |
| `vellum schema` | Write the published JSON Schema for a specification, raw and unwrapped. |
| `vellum provenance <artifact-file> [--json]` | Report an artifact's own embedded provenance record, if it carries one, without opening it in any external tool. |
| `vellum mcp` | Run a Model Context Protocol server, exposing every `vellum_*` tool over stdin/stdout as newline-delimited JSON-RPC, for an MCP client to launch as a subprocess and connect to. Runs until the connection ends. No `--json` mode: MCP is itself the wire protocol this verb speaks. |
| `vellum doctor [--dir <path>] [--json]` | Check the local environment: the built-in theme and its fonts, `VELLUM_THEME_DIR`/`VELLUM_ASSET_DIR`/`VELLUM_MAX_ASSET_BYTES`/`VELLUM_SOURCE_DATE_EPOCH`, the PDF/A sRGB ICC profile, and write permission on `--dir` (default: the current working directory). Every check runs regardless of an earlier failure; exits non-zero (`VELLUM_CLI_DOCTOR_FAILED`) when any check failed. |

Run `vellum <command> --help` for a command's exact flags; this page is the
index `--help` itself does not replace, per CLAUDE.md's Update Demand table.
