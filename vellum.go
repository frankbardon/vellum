// Package vellum is a declarative artifact emitter: spec in, document out.
//
// Vellum produces DOCX, XLSX, PPTX and PDF/A-2b files from a single generic
// block model, and fills existing OOXML templates with bound data without
// destroying the parts it does not understand. It ships as an embeddable Go
// library, a thin CLI, and an MCP server with an embedded skill pack. The
// library is the primary deliverable; the CLI is an adapter over it.
//
// # Determinism
//
// Identical inputs produce byte-identical outputs. This is a hard requirement
// rather than an aspiration, and it is why there is no converter subprocess
// anywhere on the render path: conversion output varies with renderer version
// and with the fonts installed on the converting machine, which would defeat
// both golden-file testing and any consumer that dedupes on content.
//
// # Identity before render
//
// An artifact's name derives from the spec hash and the asset hashes, both of
// which are inputs. The name is therefore knowable before the render runs — a
// consumer that could only learn an artifact's identity by producing it could
// not use identity to avoid producing it.
//
// # Three content models
//
// The spec is unresolved and hashable; the fragment IR is resolved and
// format-neutral, so theme, font, number and asset resolution happen once and
// are shared by every writer; the per-format models are resolved and laid out,
// and are public so a consumer needing format-specific reach has it.
//
// # Status
//
// Early construction. The packages described above are being built epic by
// epic; see CLAUDE.md for the architecture and the conventions that govern it.
package vellum
