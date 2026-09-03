# Vellum

A declarative artifact emitter for Go: spec in, document out. DOCX, XLSX, PPTX
and PDF/A-2b from one generic block model, with byte-identical output — plus a
fill mode that binds data into an existing OOXML template without destroying the
parts it does not understand.

This site documents the CLI, library embedding, the spec format and the
internals, for human readers. LLM-facing guidance lives in the embedded skill
pack under `skills/` and is loaded via MCP at runtime.

The library, the CLI (`vellum compose`/`validate`/`fill`/`inspect`/`boxes`/
`capabilities`/`schema`/`provenance`/`mcp`/`doctor`), the MCP server and its
ten tools, the embedded skill pack, and a runnable example pack all exist and
are tested today — start at [Installation](getting-started/installation.md)
or [Quickstart](getting-started/quickstart.md).
