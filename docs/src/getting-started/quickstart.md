# Quickstart

This composes a small, realistic report — a heading, a paragraph, an embedded
logo, a table and a page break — to a `.docx` file, using nothing but the
library facade and the built-in theme. It is a lightly adapted version of
[`examples/format-docx.json`](https://github.com/frankbardon/vellum/blob/main/examples/format-docx.json),
one of the runnable specifications shipped in Vellum's own example pack — see
[the MCP tool surface](../mcp/index.md) for how an agent reads the same file
through `vellum_examples`.

## The specification

A [`spec.Spec`](../spec/blocks.md) is JSON or YAML, decoded strictly — an
unrecognised field is a build failure, never a silent skip. Save this as
`report.json`:

```json
{
  "format_version": "1.0",
  "title": "Quarterly Business Review",
  "sections": [
    {
      "id": "overview",
      "blocks": [
        { "kind": "heading", "heading": { "level": 1, "content": "Quarterly Business Review" } },
        {
          "kind": "text",
          "text": {
            "content": "This report summarises performance for the quarter just ended, with detail on revenue by region."
          }
        },
        {
          "kind": "asset",
          "asset": {
            "handle": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=",
            "alt_text": "Company logo"
          }
        },
        { "kind": "spacer", "spacer": { "height": { "value": 12, "unit": "pt" } } }
      ]
    },
    {
      "id": "results",
      "blocks": [
        { "kind": "heading", "heading": { "level": 2, "content": "Results by Region" } },
        {
          "kind": "table",
          "table": {
            "caption": "Revenue by region",
            "column_headers": [
              { "label": "Region" },
              { "label": "Revenue (USD)" }
            ],
            "body": [
              [ { "value": { "kind": "text", "text": "Americas" } }, { "value": { "kind": "number", "number": 812000 } } ],
              [ { "value": { "kind": "text", "text": "EMEA" } }, { "value": { "kind": "number", "number": 501000 } } ],
              [ { "value": { "kind": "text", "text": "APAC" } }, { "value": { "kind": "number", "number": 276000 } } ]
            ]
          }
        },
        { "kind": "page_break", "page_break": {} },
        { "kind": "text", "text": { "content": "Detailed regional commentary continues on the next page." } }
      ]
    }
  ]
}
```

The `asset.handle` here is a `data:` URI, which the default inline asset
resolver serves without any configuration — see [Seams](../library/seams.md)
for how to serve real stored assets instead.

## Composing it in Go

```go
package main

import (
	"context"
	"encoding/json"
	"os"

	"github.com/frankbardon/vellum"
	"github.com/frankbardon/vellum/artifact"
	"github.com/frankbardon/vellum/spec"
)

func main() {
	raw, err := os.ReadFile("report.json")
	if err != nil {
		panic(err)
	}

	var s spec.Spec
	if err := json.Unmarshal(raw, &s); err != nil {
		panic(err)
	}

	v, err := vellum.New(vellum.Options{})
	if err != nil {
		panic(err)
	}

	out, err := os.Create("report.docx")
	if err != nil {
		panic(err)
	}
	defer out.Close()

	report, err := v.Compose(context.Background(), &s, artifact.FormatDOCX, out)
	if err != nil {
		panic(err)
	}
	for _, w := range report.Warnings {
		println("warning:", w.Error())
	}
}
```

`vellum.New(vellum.Options{})` is the zero-configuration path: every seam
(theme provider, asset resolver, FEEL evaluator) defaults to something that
works, per [Seams](../library/seams.md). `Compose` resolves the spec against
DOCX's theme and capability matrix, lowers it into the DOCX model, and writes
it — nothing reaches `out` until resolution has succeeded, so a rejected
specification never produces a half-written file.

Swap `artifact.FormatDOCX` for `artifact.FormatXLSX`, `artifact.FormatPPTX` or
`artifact.FormatPDF` to render the same specification to a different format —
see [Formats](../formats/capabilities.md) for what each one does with a
`page_break`, a table, and an image differently.

## The same thing from the CLI

```sh
vellum compose report.json --format docx -o report.docx
```

Or validate it first, against every format, without writing anything:

```sh
vellum validate report.json --format pdf
```

This one will fail against the built-in theme, honestly: the built-in theme's
three faces are all non-embeddable, and PDF/A-2b requires every font embedded,
so no specification composes to PDF against it
(`VELLUM_FONT_EMBED_UNSUPPORTED`) until a theme with an embeddable face is
supplied. See [PDF and PDF/A](../formats/pdf.md).

See [Commands and flags](../cli/flags.md) for the full command index, and
[The Spec](../spec/blocks.md) for every block kind this specification could
have used.
