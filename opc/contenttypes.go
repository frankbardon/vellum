package opc

import (
	"encoding/xml"
	"sort"
	"strings"

	verr "github.com/frankbardon/vellum/errors"
)

const ctNamespace = "http://schemas.openxmlformats.org/package/2006/content-types"

// Default declares the content type for every part with a given extension.
type Default struct {
	Extension   string
	ContentType string
}

// Override declares the content type for one specific part, taking precedence
// over any matching default.
type Override struct {
	PartName    string
	ContentType string
}

// ContentTypes is the [Content_Types].xml declaration.
//
// Ordered slices rather than maps, for the same reason [Relationships] is: the
// emitted order is part of the bytes, and a map would make it depend on Go's
// hash seed.
type ContentTypes struct {
	defaults  []Default
	overrides []Override

	parsed bool
	raw    []byte
	dirty  bool
}

// SetDefault declares the content type for an extension, replacing any
// existing declaration for it.
func (c *ContentTypes) SetDefault(ext, contentType string) {
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))
	for i := range c.defaults {
		if c.defaults[i].Extension == ext {
			if c.defaults[i].ContentType != contentType {
				c.defaults[i].ContentType = contentType
				c.dirty = true
			}
			return
		}
	}
	c.defaults = append(c.defaults, Default{Extension: ext, ContentType: contentType})
	c.dirty = true
}

// SetOverride declares the content type for one part, replacing any existing
// declaration for it.
func (c *ContentTypes) SetOverride(name, contentType string) {
	for i := range c.overrides {
		if c.overrides[i].PartName == name {
			if c.overrides[i].ContentType != contentType {
				c.overrides[i].ContentType = contentType
				c.dirty = true
			}
			return
		}
	}
	c.overrides = append(c.overrides, Override{PartName: name, ContentType: contentType})
	c.dirty = true
}

// removeOverride drops the declaration for name, if present.
func (c *ContentTypes) removeOverride(name string) {
	for i := range c.overrides {
		if c.overrides[i].PartName == name {
			c.overrides = append(c.overrides[:i], c.overrides[i+1:]...)
			c.dirty = true
			return
		}
	}
}

// ContentTypeFor resolves the declared content type for a part name: an
// override if one exists, otherwise the default for its extension.
func (c *ContentTypes) ContentTypeFor(name string) (string, bool) {
	if c == nil {
		return "", false
	}
	for _, o := range c.overrides {
		if o.PartName == name {
			return o.ContentType, true
		}
	}
	ext := extension(name)
	for _, d := range c.defaults {
		if d.Extension == ext {
			return d.ContentType, true
		}
	}
	return "", false
}

// Defaults returns a copy of the default declarations.
func (c *ContentTypes) Defaults() []Default {
	if c == nil {
		return nil
	}
	out := make([]Default, len(c.defaults))
	copy(out, c.defaults)
	return out
}

// Overrides returns a copy of the override declarations.
func (c *ContentTypes) Overrides() []Override {
	if c == nil {
		return nil
	}
	out := make([]Override, len(c.overrides))
	copy(out, c.overrides)
	return out
}

// marshal serialises the declaration. A parsed, unmutated declaration returns
// its original bytes, for the same round-trip reason relationships do.
func (c *ContentTypes) marshal() []byte {
	if c.parsed && !c.dirty {
		return c.raw
	}

	// Sort a generated declaration so the output is a function of the content
	// rather than of the order parts happened to be added. A parsed one keeps
	// its original order, which is only reachable here if it was mutated —
	// and in that case the file is being rewritten anyway.
	defaults := make([]Default, len(c.defaults))
	copy(defaults, c.defaults)
	overrides := make([]Override, len(c.overrides))
	copy(overrides, c.overrides)
	if !c.parsed {
		sort.Slice(defaults, func(i, j int) bool { return defaults[i].Extension < defaults[j].Extension })
		sort.Slice(overrides, func(i, j int) bool { return overrides[i].PartName < overrides[j].PartName })
	}

	var b strings.Builder
	b.WriteString(xmlDeclaration)
	b.WriteString(`<Types xmlns="`)
	b.WriteString(ctNamespace)
	b.WriteString(`">`)
	for _, d := range defaults {
		b.WriteString(`<Default Extension="`)
		b.WriteString(escapeAttr(d.Extension))
		b.WriteString(`" ContentType="`)
		b.WriteString(escapeAttr(d.ContentType))
		b.WriteString(`"/>`)
	}
	for _, o := range overrides {
		b.WriteString(`<Override PartName="`)
		b.WriteString(escapeAttr(o.PartName))
		b.WriteString(`" ContentType="`)
		b.WriteString(escapeAttr(o.ContentType))
		b.WriteString(`"/>`)
	}
	b.WriteString(`</Types>`)
	return []byte(b.String())
}

type ctXML struct {
	XMLName  xml.Name `xml:"Types"`
	Defaults []struct {
		Extension   string `xml:"Extension,attr"`
		ContentType string `xml:"ContentType,attr"`
	} `xml:"Default"`
	Overrides []struct {
		PartName    string `xml:"PartName,attr"`
		ContentType string `xml:"ContentType,attr"`
	} `xml:"Override"`
}

func parseContentTypes(data []byte) (*ContentTypes, error) {
	var doc ctXML
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, verr.WrapCodedError(err, verr.VELLUM_OPC_INVALID,
			"[Content_Types].xml is not well-formed XML")
	}

	c := &ContentTypes{parsed: true, raw: data}
	for _, d := range doc.Defaults {
		c.defaults = append(c.defaults, Default{
			Extension:   strings.ToLower(d.Extension),
			ContentType: d.ContentType,
		})
	}
	for _, o := range doc.Overrides {
		c.overrides = append(c.overrides, Override{PartName: o.PartName, ContentType: o.ContentType})
	}
	return c, nil
}
