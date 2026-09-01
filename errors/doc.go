// Package errors provides the structured error codes and coded error type used
// throughout Vellum.
//
// Vellum does no logging. Diagnostics are returned as data: a [CodedError]
// carries a machine-readable [Code], a human-readable message, and a
// structured detail map that names the offending block, part, anchor or
// coordinate. Callers that need to branch do so on the code; callers that need
// to render do so from the details.
//
// Every code carries a [Metadata] row giving its canonical message and either
// at least one [Fixup] — an imperative hint at what the author should change —
// or an explicit FixupNotApplicable flag for the internal invariants no author
// can act on. The pairing is enforced by TestCodesHaveFixups, so a code added
// without guidance fails the build rather than reaching a user.
package errors
