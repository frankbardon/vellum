package xmlcopy_test

// Namespace URIs used across fixtures and assertions, matching the ones the
// OOXML writers in this repository already declare (see doc/xml.go and
// sheet/xml.go).
const (
	nsWordprocessing = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
	nsWord14         = "http://schemas.microsoft.com/office/word/2010/wordml"
	nsMarkupCompat   = "http://schemas.openxmlformats.org/markup-compatibility/2006"
	nsCustomXML      = "http://schemas.openxmlformats.org/officeDocument/2006/customXml"
)

// wordSnippet is a realistic WordprocessingML document.xml fragment: two
// content controls, one nested inside the other; a self-closing empty
// paragraph; an entity inside run text; an xml:space="preserve" attribute;
// attributes in an order Word itself would write them (not alphabetised);
// and a mix of the w: and w14: namespace prefixes bound in the root element.
// This is what Walk and Apply have to survive intact — a hand-rolled toy
// element tree would not exercise the attribute-order and prefix cases that
// motivate this package existing at all.
const wordSnippet = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
	`<w:document xmlns:w="` + nsWordprocessing + `" xmlns:w14="` + nsWord14 +
	`" xmlns:mc="` + nsMarkupCompat + `" mc:Ignorable="w14">` +
	`<w:body>` +
	`<w:p w14:paraId="1A2B3C4D" w:rsidR="00AA1234">` +
	`<w:r><w:t xml:space="preserve">Report for </w:t></w:r>` +
	`<w:sdt>` +
	`<w:sdtPr><w:alias w:val="ClientName"/><w:tag w:val="client_name"/><w:id w:val="123456789"/></w:sdtPr>` +
	`<w:sdtContent><w:r><w:t>Acme &amp; Co.</w:t></w:r></w:sdtContent>` +
	`</w:sdt>` +
	`<w:r><w:t> — quarterly review</w:t></w:r>` +
	`</w:p>` +
	`<w:p>` +
	`<w:sdt>` +
	`<w:sdtPr><w:tag w:val="outer"/></w:sdtPr>` +
	`<w:sdtContent>` +
	`<w:sdt>` +
	`<w:sdtPr><w:tag w:val="inner"/></w:sdtPr>` +
	`<w:sdtContent><w:r><w:t>nested</w:t></w:r></w:sdtContent>` +
	`</w:sdt>` +
	`</w:sdtContent>` +
	`</w:sdt>` +
	`</w:p>` +
	`<w:p/>` +
	`<w:sectPr><w:pgSz w:w="12240" w:h="15840"/></w:sectPr>` +
	`</w:body>` +
	`</w:document>`

// cdataSnippet models a customXml data-island part, the one place raw CDATA
// legitimately turns up in an OOXML package.
const cdataSnippet = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
	`<ds:datastoreItem xmlns:ds="` + nsCustomXML + `" ds:itemID="{ABCDEF01-0000-0000-0000-000000000000}">` +
	`<ds:schemaRefs><![CDATA[raw <data> & stuff]]></ds:schemaRefs>` +
	`</ds:datastoreItem>`
