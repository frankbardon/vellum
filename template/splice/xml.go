package splice

// Namespace and relationship-type constants, this package's own copy —
// matching doc/xml.go's shapes exactly, because doc/table.go and
// doc/write_parts.go are read-only reference here (see the story brief), not
// an import: sheet and deck each carry their own copy of the same constants
// rather than sharing one, and this package follows that established
// convention.
const (
	nsWordprocessing = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
	nsRelationships  = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	nsDrawingWP      = "http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing"
	nsDrawingMain    = "http://schemas.openxmlformats.org/drawingml/2006/main"
	nsDrawingPicture = "http://schemas.openxmlformats.org/drawingml/2006/picture"
)

// relImage is the relationship type an inline picture's r:embed resolves
// through.
const relImage = nsRelationships + "/image"

// mediaExtension returns the file extension for an accepted media type. Only
// PNG and JPEG are accepted; the caller checks that before this is reached.
func mediaExtension(mediaType string) string {
	switch mediaType {
	case mediaPNG:
		return "png"
	case mediaJPEG:
		return "jpeg"
	}
	return "bin"
}

const (
	mediaPNG  = "image/png"
	mediaJPEG = "image/jpeg"
)
