# Vellum documentation

mdBook source. `make docs` builds to `docs/book/` (gitignored); `make docs-serve`
serves it locally with live reload. CI publishes it to GitHub Pages on push to
`main`, and additionally copies the payload JSON Schema to the site root so its
`$id` URL resolves.

Audience split: this site is for **humans** — CLI, embedding, format, internals.
LLM-facing guidance lives in the embedded skill pack under `skills/`, loaded via
MCP at runtime. They are not the same document written twice.
