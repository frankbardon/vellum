# The output envelope

Every `--json` CLI invocation and every MCP tool call — with one documented
exception — writes a `descriptor.Envelope`. Current `format_version` is
`"1.0"`.

```json
{
  "format_version": "1.0",
  "data": { },
  "request": { },
  "errors":   [ { "code": "VELLUM_...", "message": "...", "details": { } } ],
  "warnings": [ { "code": "VELLUM_...", "message": "...", "details": { } } ]
}
```

## The invariants that matter to a consumer

- **`errors` and `warnings` are always present as arrays** — never `null`
  and never omitted, on both a success and a failure. A consumer iterates
  either without a nil-guard, every time, unconditionally.
- **`request` is omitted when absent.** Not every call echoes its own
  request back; when one does not, the field is simply not present rather
  than present-and-null.
- **`data` is not trustworthy when `errors` is non-empty.** A caller checks
  `errors` first.
- Every code in `errors` and `warnings` matches `^VELLUM_[A-Z0-9_]+$` — the
  same domain-prefixed `errors.Code` vocabulary used everywhere else in the
  library, never a bare string a writer improvised in the moment.
- **Additive slots are `omitempty`**, so a new field added to a response
  shape does not change pre-existing wire output byte-for-byte, and adding
  one does *not* bump `format_version` — a version bump is reserved for a
  genuinely breaking change to the envelope or the wire contract, not for
  growth.

## `warnings` is where a degradation is reported

The `warnings` array is not an afterthought slot — it is the other half of
the capability matrix's `degrades` promise. CLAUDE.md states this directly:
"A `degrades` row produces a warning the consumer can actually read, and
that is machine-checked" (`TestCapabilityDegradationsAreReported`). A `heading`
block degrading to a styled XLSX cell, a font substituted because a theme's
declared face was not embeddable, a table's cell annotation becoming an
extra column — every one of these surfaces here, naming the feature that
degraded, so a caller composing an unattended batch job learns about the
substitution from the response rather than from a reader noticing an
absence weeks later. See [Capability matrix](../formats/capabilities.md).

## Where errors land

Errors always land in the envelope's `errors` array on stdout in `--json`
mode — exactly the same place a success's data would be, never on stderr.
In the CLI's default human-readable mode (no `--json`), the same information
prints to stderr instead. Every command's exit code follows one convention
regardless of mode:

| Code | Meaning |
|---|---|
| `0` | Success. |
| `1` | The command was well-formed and the operation it asked for failed — a rejected specification, an unreconciled binding. |
| `2` | The command's own invocation was malformed — a bad flag, a missing argument, a file that does not exist. Nothing was attempted. |

See [Commands and flags](../cli/flags.md).

## The one exception: `vellum schema`

`vellum schema` writes the raw JSON Schema **unwrapped**, with no envelope
around it at all, because the payload it returns is already self-contained —
it carries its own `$schema` and `$id` keywords, and wrapping it in an
envelope would nest one self-describing document inside a field of another
for no benefit. See [Payload schema](payload-schema.md). Every other command,
including `vellum_schema` over MCP (which has no unwrapped transport mode to
exploit), carries the schema as one field of a normal envelope instead.

## Every output path goes through one constructor

`descriptor.NewEnvelope` is the sole path onto the wire for both the CLI's
`--json` mode and every MCP tool response — CLAUDE.md states this as an
absolute: "no `fmt.Sprintf` builds JSON." There is exactly one place in the
codebase that decides what an envelope looks like on the wire, which is what
keeps a CLI `--json` response and an MCP tool response of the same logical
operation byte-for-byte identical in shape.

## See also

- [Payload schema](payload-schema.md) — the machine-readable JSON Schema
  this envelope, and everything it can carry, validates against.
- [Capability matrix](../formats/capabilities.md) — what actually populates
  `warnings`.
- [Commands and flags](../cli/flags.md) — `--json` at the CLI layer.
