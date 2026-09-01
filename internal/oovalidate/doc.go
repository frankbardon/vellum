// Package oovalidate runs an installed LibreOffice against artifacts Vellum
// wrote, as an optional test-only check that a real office reader accepts them.
//
// # Why this exists
//
// Vellum's determinism harness compares our bytes against our bytes. That
// catches every nondeterminism it was built for and no correctness bug at all:
// a file can be byte-identical across a thousand runs and still be one no
// office application will open. Three defects have now arrived that way — zip
// version fields left at zero, ECMA-376 child elements emitted out of schema
// order, and a golden "PNG" with no IDAT chunk. Every Go-side reader accepted
// all three. Word accepted none of them.
//
// The gap is structural: the readers available to the build are more tolerant
// than the readers that matter. This package narrows it by borrowing a reader
// that is not ours.
//
// # What it is not
//
// It is not a conversion path, and CLAUDE.md's ruling that LibreOffice is
// "permanently ruled out, in any form" is unchanged. That ruling is about
// producing artifacts: conversion output varies with renderer version and
// installed fonts, so a converted artifact defeats byte-identical output and
// the consumer dedupe resting on it. Nothing here produces an artifact Vellum
// ships. LibreOffice is used strictly as an oracle — it reads bytes we already
// wrote and reports whether it could — and its output is examined for presence
// and content, never compared for equality. Asserting anything byte-wise about
// a LibreOffice conversion would import exactly the nondeterminism the ruling
// exists to keep out.
//
// The implementation is behind the `soffice` build tag, so it cannot link into
// a build that did not ask for it, and it lives under internal/ so it is not
// public API. TestNoOfficeToolingOnTheLibraryPath proves no shipped package
// reaches it.
//
// # Honest limits
//
// LibreOffice is its own reader with its own tolerances, and it is not Word.
// It did not object to any of the three defects above except by accident, and a
// pass here is evidence, not proof. Specifically:
//
//   - It is more tolerant than Word about zip header fields and about several
//     ECMA-376 ordering constraints. A pass does not mean Word will open it.
//   - It is less tolerant than Word about some legitimate constructs, so a
//     failure needs reading before it is believed.
//
// It is worth having anyway, because the failures it does catch — a package it
// cannot open at all, content that silently vanished — are the expensive ones,
// and today nothing catches them before a human opens the file by hand.
package oovalidate
