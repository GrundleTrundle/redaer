package store

/*
 Feed discovery and data extraction.
*/
import (
	"encoding/xml"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"redaer/links"
)

var contentType = http.CanonicalHeaderKey("Content-type")

// Tries to determine the url for a feed descriptor based on the
// contents of a page. Should return transientError for connection
// errors that could be temporary.
func findFeedUrl(client *http.Client, ld *LinkDetails) (string, error) {
	log.Printf("Looking for feed for (%s)\n", ld.Title)
	resp, err := client.Get(ld.BaseUrl)
	if err != nil {
		// Err on conservative side, and classify all these as transient.
		return "", MkTransientError("findFeedUrl: %s", err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == 200:
		links, err := links.Extract(resp.Body)
		if err != nil {
			// Extract() can return partial results on parse errors.  Try to live with it,
			// and warn if we got back some links. (hope for a <link rel="alternate"> that has
			// the info we need).
			if len(links) == 0 {
				return "", MkTransientError("findFeedUrl extract: %s", err)
			}
			log.Printf("\tWARNING: errors parsing (%s) page for feed links: %s", ld.Title, err)
		}
		for _, link := range links {
			//log.Printf(".....check %#v\n", link)
			feedUrl, ok := checkForFeedUrl(client, ld.BaseUrl, link)
			if ok {
				return feedUrl, nil
			}
		}

		// No likely link found.
		return "", MkTransientError("findFeedUrl: No link found in main page for %s", ld.Title)
	case resp.StatusCode >= 500:
		return "", MkTransientError("findFeedUrl: Server error %s", resp.Status)
	default:
		return "", MkError("Error %d reading %s", resp.StatusCode, ld.BaseUrl)
	}
}

var feedHints []string = []string{"rss", "atom", "feed"}
var urlSuffixes []string = []string{"atom.xml", "rss.xml", "feed.xml",
	"feed=rss2", "feed=atom", "feed=rss", "feed"}

func namedLikeFeedLink(link links.Link) bool {
	t := strings.ToLower(link.Title)
	for _, cand := range feedHints {
		if strings.Contains(t, cand) {
			//log.Printf("   match hint %s=%s", cand, t)
			return true
		}
	}

	u := strings.ToLower(link.Url)
	for _, cand := range urlSuffixes {
		if u == cand || strings.HasSuffix(u, cand) {
			//log.Printf("   match url %s=%s", cand, u)
			return true
		}
	}
	return false
}

func checkForFeedUrl(client *http.Client, baseUrl string, link links.Link) (url string, ok bool) {
	if !namedLikeFeedLink(link) {
		return "", false
	}

	resp, err := makeFeedRequest(client, baseUrl, link.Url)
	if err != nil {
		log.Printf("\tcheckForFeedUrl: %s\n", err)
		return "", false
	}
	defer resp.Body.Close()

	if validFeedContentType(resp.Header[contentType]) &&
		recognizedFeedFormat(resp.Body) {
		return link.Url, true
	}

	return "", false
}

// If maybeRel is relative, 
func forceAbsolute(base, maybeRel string) (string, error) {
	maybeRelU, err := url.Parse(maybeRel)
	if err != nil {
		return "", err
	}
	baseU, err := url.Parse(base)
	if err != nil {
		return "", err
	}

	actualU := baseU.ResolveReference(maybeRelU)
	return actualU.String(), nil
}

func makeFeedRequest(client *http.Client, baseUrl, reqUrl string) (*http.Response, error) {
	absU, err := forceAbsolute(baseUrl, reqUrl)
	if err != nil {
		return nil, err
	}

	resp, err := client.Get(absU)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, MkError("Bad response: %s", resp.Status)
	}

	return resp, nil
}

var contentTypes = []string{"text/xml", "text/plain", "application/xml", "application/rss+xml", "application/atom+xml"}

func validFeedContentType(ct []string) bool {
	for _, c := range ct {
		for _, pref := range contentTypes {
			if strings.HasPrefix(c, pref) {
				return true
			}
		}
	}
	return false
}

func recognizedFeedFormat(rd io.Reader) bool {
	ring := xml.NewDecoder(rd)
	start, err := firstStartElement(ring)

	if err == nil {
		return parseFunctionForFeed(start) != nil
	}
	return false
}

func firstStartElement(ring *xml.Decoder) (xml.StartElement, error) {
	var err error
	var t xml.Token

	for err == nil {
		t, err = ring.Token()

		if err == nil {
			t, ok := t.(xml.StartElement)
			if ok {
				return t, nil
			}
		}
	}

	return xml.StartElement{}, MkError("No start element found.")
}

// Have to wait 5 minutes between checks of a given link.
const (
	minCheckDuration = 300.0e9
)

func checkForArticles(client *http.Client, link *LinkDetails) {
	log.Printf("Checking for articles: (%s)\n", link.Title)
	link.Articles = make([]Article, 0)

	moment := time.Now()
	timeSinceLast := moment.Sub(link.LastChecked)
	if timeSinceLast < minCheckDuration {
		log.Printf("\tLess than %s since the last check, skipping.", time.Duration(minCheckDuration).String())
		return
	}

	resp, err := makeFeedRequest(client, link.BaseUrl, link.FeedUrl)
	if err != nil {
		log.Printf("checkForArticles: %s\n", err)
		link.ErrorOccurred(err)
		return
	}
	defer resp.Body.Close()

	ring := xml.NewDecoder(resp.Body)
	first, err := firstStartElement(ring)
	if err != nil {
		log.Printf("checkForArticles startElement: %s\n", err)
		link.ErrorOccurred(err)
		return
	}

	pfn := parseFunctionForFeed(first)
	if pfn != nil {
		err = pfn(ring, first, link)
		if err == nil {
			if len(link.Articles) > 1 {
				sort.Sort(link.Articles)
			}
			// Guard against relative urls in links.
			for i, art := range link.Articles {
				url, err := forceAbsolute(link.BaseUrl, art.Url)
				if err == nil {
					link.Articles[i].Url = url
 				} else {
					link.ErrorOccurred(err)
					link.Articles = make([]Article, 0)
					return
				}
			}

			// Only update this when we succeeded.
			link.LastChecked = moment
		} else {
			link.ErrorOccurred(err)
			link.Articles = make([]Article, 0)
		}
	}
}

// Parses a feed format on a stream that has already consumed the root StartElement.
type ArticleParser func(ring *xml.Decoder, root xml.StartElement, link *LinkDetails) error

func parseFunctionForFeed(root xml.StartElement) ArticleParser {
	tag := root.Name.Local

	//log.Printf("   Feed root: %s:%s\n", root.Name.Space, root.Name.Local)
	switch {
	case tag == "rss":
		return feedParser
	case tag == "feed":
		return feedParser

	case tag == "RDF":
		// WTF.
		return feedParser
	default:
		return nil
	}
}

func feedParser(ring *xml.Decoder, root xml.StartElement, link *LinkDetails) error {
	for {
		t, err := ring.Token()

		if err == io.EOF {
			return nil
		} else if err != nil {
			return err
		}

		switch t := t.(type) {
		case xml.StartElement:
			// Limitation: we ignore "channels" in the RSS and just pull all articles.
			looksLikeAtom := t.Name.Local == "entry"
			if t.Name.Local == "item" || looksLikeAtom {
				art, err := genericExtractArticle(ring, link, looksLikeAtom)
				if err != nil {
					return err
				}
				link.Articles = append(link.Articles, art)
			}
		}
	}
}

func genericExtractArticle(ring *xml.Decoder, link *LinkDetails, looksLikeAtom bool) (Article, error) {
	var accum string
	rv := Article{}
	for {
		t, err := ring.Token()
		if err != nil {
			return Article{}, err
		}

		switch t := t.(type) {
		case xml.StartElement:
			accum = ""
			if looksLikeAtom && t.Name.Local == "link" {
				// Extract from href attribute instead of body.
				found := false
				for _, attr := range t.Attr {
					if attr.Name.Local == "href" {
						rv.Url = attr.Value
						found = true
					}
				}
				if !found {
					return Article{}, MkError("Could not find HREF attribute for ENTRY LINK.")
				}
			}

		case xml.EndElement:
			switch t.Name.Local {
			case "item", "entry":
				// End of the item, we should be done.
				return rv, nil
			case "title":
				// Hack.  Some feeds have a more than one title tag using different namespaces.
				// (in the src xml file, there's <title> which is the one we want, but also a
				//  media:title, which just has a username in it.  That shadows the title we want).
				// Just appending to the title field here for now.  A real fix would have it look
				// at the xmlns attribute in the root tag, and then match against that.
				rv.Title += accum
			case "link":
				if !looksLikeAtom {
					rv.Url = accum
				}
			case "pubDate", "updated", "date":
				tm, err := parseTime(accum)
				if err != nil {
					return rv, err
				}
				rv.PubDate = tm
			}
		case xml.CharData:
			accum += string(t)
		}
	}
}

// The spec says RFC822 or 822Z, but I see feeds with other formats as
// well, so we try a few things here.
func parseTime(accum string) (time.Time, error) {
	formats := []string{time.RFC822, time.RFC822Z, time.RFC1123, time.RFC1123Z,
		time.RFC3339, "2006-1-2"}

	for _, format := range formats {
		tm, err := time.Parse(format, accum)
		if err == nil {
			return tm, nil
		}
	}

	return time.Time{}, MkError("Could not parse time: %s", accum)
}
