# Hostile seed corpus

Inputs for the fuzz targets over `opc.Open` and the `zipdet` reader, and for
the table test that pins each one's expected outcome.

These parse untrusted input the moment a consumer accepts a user-supplied
template, so the bar is not "handles the happy path" — it is that **every input
produces either a valid package or a coded error, and never a panic, an
unbounded allocation, or a write outside the package.**

| File | What it is | Expected |
|---|---|---|
| `valid-minimal.zip` | A well-formed three-part package | opens |
| `traversal-entry-name.zip` | Entry named `../../etc/passwd` | `VELLUM_ZIP_ENTRY_NAME_INVALID` — refused, never sanitised |
| `missing-content-types.zip` | No `[Content_Types].xml` | `VELLUM_OPC_INVALID` |
| `malformed-rels.zip` | Relationships part that is not well-formed XML | `VELLUM_OPC_RELATIONSHIP_INVALID` |
| `rels-missing-target.zip` | A relationship with no `Target` attribute | `VELLUM_OPC_RELATIONSHIP_INVALID` |
| `dangling-relationship.zip` | A relationship pointing at a part that is absent | **opens**, and `Package.Validate` rejects it |
| `bomb.zip` | 32 MiB of zeros in a 33 KB archive | `VELLUM_ZIP_TOO_LARGE` under a bound |
| `truncated.zip` | Half a valid archive | `VELLUM_ZIP_MALFORMED` |
| `not-a-zip.zip` | Plain text | `VELLUM_ZIP_MALFORMED` |
| `empty.zip` | Zero bytes | `VELLUM_ZIP_MALFORMED` |

The dangling-relationship case is the one worth understanding. It opens on
purpose: a package Vellum did not build may legitimately reference a part it
does not carry, and refusing to open such a file would leave fill mode unable
to inspect the documents it exists to work with. Reading is permissive about
semantic consistency; writing is not, and `Package.Validate` is where that line
sits.

## Adding a case

Drop the archive in this directory, add a row to the table above, and add an
entry to `seedExpectations` in `opc/seeds_test.go`. A file present here without
an expectation fails the build, so a seed cannot be added and quietly not
exercised.

Real-world templates are welcome and more valuable than synthetic ones —
particularly anything produced by a tool other than Word.
