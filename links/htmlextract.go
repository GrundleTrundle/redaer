package links

import (
	"fmt"
	"io"

	"golang.org/x/net/html"
)

// Link extracted from a HTML file.
type Link struct {
	Title string
	Url   string
}

// Extracts links from LINK and A tags from html source.
// If an error occurs, all of the links parsed up to the error
// position are returned as well.
func Extract(in io.Reader) ([]Link, error) {
	parser := html.NewTokenizer(in)
	rv := make([]Link, 0)
	var err error
	for {
		ty := parser.Next()
		switch ty {
		case html.ErrorToken:
			err = parser.Err()
			if err == io.EOF {
				return rv, nil
			} else {
				return rv, err
			}
		case html.SelfClosingTagToken, html.StartTagToken:
			tok := parser.Token()
			rv, err = checkTag(tok, parser, rv)
			if err != nil {
				return rv, err
			}
		}
	}
}

func checkTag(startTok html.Token, parser *html.Tokenizer, links []Link) ([]Link, error) {
	if startTok.Data == "a" && startTok.Type == html.StartTagToken {
		return checkForAnchor(startTok, parser, links)
	} else if startTok.Data == "link" {
		lnk, ok := checkForLINK(startTok)
		if ok {
			return append(links, lnk), nil
		}
	}
	return links, nil
}

func checkForAnchor(startTok html.Token, parser *html.Tokenizer, links []Link) ([]Link, error) {
	if startTok.Data != "a" {
		return links, nil
	}

	href, ok := findAttr(startTok.Attr, "href")
	if !ok {
		return links, nil
	}

	lnk := Link{Url: href.Val}
	depth := 0

tokLoop:
	for {
		ty := parser.Next()
		switch ty {
		case html.ErrorToken:
			err := parser.Err()
			if err == io.EOF {
				return links, fmt.Errorf("EOF while parsing anchor body.")
			}
			return links, err

		case html.StartTagToken:
			depth++

		case html.EndTagToken:
			depth--
			// At this point, we're at the right level. If the HTML
			// is mismatched, and this isn't an 'a' tag, we still want
			// to break out here.  Not treating it as an error for now.
			if depth < 0 {
				break tokLoop
			}

		case html.TextToken:
			tok := parser.Token()
			lnk.Title += tok.Data
		}
	}

	// If there's no text data, check for title attribute as a last ditch effort.
	if lnk.Title == "" {
		title, ok := findAttr(startTok.Attr, "title")
		if ok {
			lnk.Title = title.Val
		} else {
			return links, nil
		}
	}
	return append(links, lnk), nil
}

func checkForLINK(tok html.Token) (Link, bool) {
	if tok.Data != "link" {
		return Link{}, false
	}

	href, ok := findAttr(tok.Attr, "href")
	if !ok {
		return Link{}, false
	}

	rv := Link{Url: href.Val,
		Title: "Some Link"}
	tyattr, ok := findAttr(tok.Attr, "type")
	if ok {
		rv.Title = tyattr.Val
	}
	return rv, true
}

func findAttr(attrs []html.Attribute, name string) (html.Attribute, bool) {
	for _, att := range attrs {
		if att.Key == name {
			return att, true
		}
	}
	return html.Attribute{}, false
}
