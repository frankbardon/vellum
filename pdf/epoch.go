package pdf

import "time"

// PinnedEpoch is the timestamp a write uses when none is supplied.
//
// The same instant the OPC writer pins, so a DOCX and a PDF composed from one
// specification carry the same date rather than two arbitrary ones that happen
// to have been chosen in different packages.
var PinnedEpoch = time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC)
