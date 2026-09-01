// Package descriptor is Vellum's output contract and self-description.
//
// It holds the envelope every JSON output is wrapped in, the manifest that
// describes what Vellum can do, and the published payload schema. Together
// they are what lets an agent discover the library rather than be told about
// it.
//
// # No-execute
//
// This package must not import a renderer. It describes; it never produces.
// That is what makes "what can Vellum do" a cheap question — answerable
// without a writer, a theme provider or an asset resolver anywhere in
// sight — and it is enforced by an import-firewall test rather than by
// convention.
package descriptor
