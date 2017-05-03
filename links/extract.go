package links

import (
	"encoding/xml"
	"fmt"
	"io"
)

// Link extracted from a HTML file.
type Link struct {
	Title string
	Url   string
}

// Extracts all links from HTML produced by SRC.  Returns
// the complete list, or just an error if there is a problem
// parsing.
// We recognize the xml parser sucks at parsing some in the wild
// HTML, and so will return partial results even when we have
// a parse error.
func Extract(src io.Reader) (v []Link, err error) {
	ring := xml.NewDecoder(src)
	ring.Strict = false
	ring.AutoClose = xml.HTMLAutoClose
	ring.Entity = xml.HTMLEntity

	return parseLinks(ring)
}

func parseLinks(d *xml.Decoder) (v []Link, err error) {
	rv := make([]Link, 0)
	for {
		t, err := d.Token()

		if err == io.EOF {
			return rv, nil
		}

		if err != nil {
			return rv, err
		}

		switch t := t.(type) {
		case xml.StartElement:
			if t.Name.Local == "a" || isLinkTagWithHREF(t) {
				lnk, err := parseLink(d, t)
				if err != nil {
					return rv, err
				}
				rv = append(rv, lnk)
			}
		}
	}
}

func extractAttr(t xml.StartElement, name string) string {
	for _, attr := range t.Attr {
		if attr.Name.Local == name {
			return attr.Value
		}
	}

	return ""
}

func extractHREF(t xml.StartElement) string {
	return extractAttr(t, "href")
}

func isLinkTagWithHREF(t xml.StartElement) bool {
	return t.Name.Local == "link" && extractHREF(t) != ""
}

func parseLink(d *xml.Decoder, anchor xml.StartElement) (Link, error) {

	url := extractHREF(anchor)
	if url == "" {
		return Link{}, fmt.Errorf("No href attribute (%s)", anchor)
	}

	title := "Some Lunk"
	var err error

	if anchor.Name.Local == "link" {
		title = extractAttr(anchor, "type")
	} else {
		title, err = extractTitleCData(d)
		if err != nil {
			return Link{}, fmt.Errorf("Could not extract title for %s", url)
		}
	}

	return Link{Title: title, Url: url}, nil
}

func extractTitleCData(d *xml.Decoder) (string, error) {
	depth := 0
	rv := ""
	for {
		t, err := d.Token()
		if err != nil {
			return "", fmt.Errorf("Error extracting title: %s", err)
		}

		switch t := t.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
			if depth == -1 && t.Name.Local == "a" {
				return rv, nil
			}
			if depth < 0 {
				return "", fmt.Errorf("Mismatched close tags for anchor?")
			}

		case xml.CharData:
			// We're collecting cdata at all depths, so something like
			// "Jorb <abbr title="A Big Toad">ABT</abbr>" would be elided to
			// "Jorb ABT".
			rv += string(t)
		}
	}
}
