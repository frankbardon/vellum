package artifact

import verr "github.com/frankbardon/vellum/errors"

// Report summarises what producing an artifact returned, beyond the bytes
// themselves.
//
// It exists so a caller composing unattended can learn what happened without
// parsing the resolved model. The public facade's Compose method returns one
// carrying every warning the resolve pass raised while producing it — a
// degraded feature, a substituted font, a mark the theme does not style —
// because a consumer scheduling a job needs those back, not only a success or
// a failure.
type Report struct {
	// Format is the format actually written.
	Format Format

	// Warnings are the coded warnings raised while producing this artifact,
	// sorted by code — the same order the resolve pass itself already sorts
	// them in, because they reach the envelope and the envelope is compared
	// byte for byte.
	//
	// Nil when the artifact was written directly from a caller-built format
	// model rather than composed from a specification: that path never runs
	// resolution, which is the only place a warning is raised.
	Warnings []*verr.CodedError
}
