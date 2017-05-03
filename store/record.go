package store

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"
)

type LinkState int

const (
	// Newly created link, we don't know if it has a feed url associated with it or not.
	LinkIsNew LinkState = iota
	// We looked, but could not find a feed for this link.
	LinkNoFeedFound
	// We had an error updating the feed, but it's expected to be a temporary error.
	LinkTransientError
	// We found a FeedUrl and recorded it.
	LinkHasFeed
	// Marked by the user as a feed we should ignore.
	LinkIgnore
)

type Article struct {
	//TODO: more fields here.
	Url     string
	Title   string
	PubDate time.Time
}
type ArticleArray []Article

type LinkDetails struct {
	// Url extracted from our source HTML.
	BaseUrl string
	// How much do we know about this link?
	State LinkState
	// If State == LinkHasFeed, this contains the URL for the feed xml file.
	FeedUrl string
	// Last error encountered if State == LinkTransientError
	LastError error `json:"-"`
	// Title presented to the user for this link.
	Title string
	// Time of last read article for this link.  Defaults to the time.Time zero value
	// for new feeds.
	LastRead time.Time
	// Last time we checked for new articles.
	LastChecked time.Time
	// All of the known articles seen at the last CheckUpdates() call, in ascending order by
	// PubDate.
	Articles ArticleArray `json:"-"`
}

type LinkStore struct {
	// Map from BaseUrl to LinkDetails for all the links we know about.  This includes
	// ones that we've seen before, but ended up not having a feed we could find.
	Links map[string]*LinkDetails

	// BaseUrl for links we are interested in this session.
	noted []string

	client *http.Client
}

type ArticleClass int

const (
	AllArticles ArticleClass = iota
	UnreadArticles
)

// Loads a link store from the given path.  Just creates a new store
// if the file doesn't exist.  Returns an error if the file exists, but
// can not be read.
//
// The life cycle of a LinkStore is:
//    s, err := Load("")
//    for url := range configured {
//       s.InterestedIn(url)
//    }
//    s.CheckForUpdates()
//    for url, details := range s.Links {
//       for linkInfo := range details.Articles {
//           ...
//       }
//   }
//   s.Save()
func Load(path string) (*LinkStore, error) {
	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return newStore(), nil
		}
		return nil, MkError("store.Load: %s", err)
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	rv := newStore()

	ring := json.NewDecoder(file)
	err = ring.Decode(rv)
	if err != nil {
		return nil, err
	}
	return rv, nil
}

func (ld *LinkDetails) UnreadArticles() []Article {
	rv := make([]Article, 0)
	for _, art := range ld.Articles {
		if ld.LastRead.Before(art.PubDate) {
			rv = append(rv, art)
		}
	}

	return rv
}

func (ld *LinkDetails) MarkAllAsRead() {
	var latest time.Time
	for _, art := range ld.Articles {
		if art.PubDate.After(latest) {
			latest = art.PubDate
		}
	}

	if latest.After(ld.LastRead) {
		ld.LastRead = latest
	}
}

//TODO: save to tmp path or []byte, and only overwrite original if everything
// encodes correctly.
func (ls *LinkStore) Save(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	enc := json.NewEncoder(file)
	//enc.SetIndent("  ", "    ") Which version was this added in?
	err = enc.Encode(ls)
	return err
}

// Creates a new clean store.
func newStore() *LinkStore {
	return &LinkStore{Links: map[string]*LinkDetails{},
		client: &http.Client{},
		noted:  make([]string, 0)}
}

// Can be called for any link that we want to check for articles.
// If the url hasn't been seen before, it will be added as a new
// entry.  Otherwise, the version in the store is left untouched.
func (ls *LinkStore) InterestedIn(url, title string) {
	_, ok := ls.Links[url]
	if !ok {
		ls.Links[url] = &LinkDetails{BaseUrl: url,
			State:    LinkIsNew,
			Articles: make([]Article, 0),
			Title:    title}
	}
	ls.noted = append(ls.noted, url)
}

// Checks the set of links InterestedIn() has been called for this
// session for new articles.
func (store *LinkStore) CheckForUpdates() {
	const NumConcurrent = 8

	linkIn := make(chan *LinkDetails, NumConcurrent)
	acks := make(chan int, len(store.noted))

	// Start the update routines.
	for i := 0; i < NumConcurrent; i++ {
		go checkLinkRoutine(store.client, linkIn, acks)
	}

	// Feed the update routines the links.
	for _, base := range store.noted {
		linkIn <- store.Links[base]
	}

	// Close the link channel so the routines know no
	// more work is coming.
	close(linkIn)

	// Wait for all routines to finish by receiving acks.
	for i := 0; i < len(store.noted); i++ {
		<-acks
	}
}

func checkLinkRoutine(client *http.Client, linkIn <-chan *LinkDetails, acks chan<- int) {
	for ld := range linkIn {
		checkLink(client, ld)
		acks <- 1
	}
}

// Updates available articles for given link.
// If the link is new, finds the feed URL as well.
func checkLink(client *http.Client, ld *LinkDetails) {
	stateNeedsCheck := true
	for stateNeedsCheck {
		stateNeedsCheck = false
		switch ld.State {
		case LinkTransientError:
			// Last time through we couldn't connect. If we have a FeedURL, assume it's
			// valid, and go back to the appropriate state.
			if ld.FeedUrl == "" {
				ld.State = LinkIsNew
			} else {
				ld.State = LinkHasFeed
			}
			stateNeedsCheck = true
		case LinkIsNew:
			feedUrl, err := findFeedUrl(client, ld)
			if IsTransient(err) {
				ld.ErrorOccurred(err)
			} else if err != nil {
				ld.State = LinkNoFeedFound
			} else {
				ld.State = LinkHasFeed
				ld.FeedUrl = feedUrl
				stateNeedsCheck = true
			}
		case LinkHasFeed:
			checkForArticles(client, ld)
		case LinkIgnore:
			log.Printf("Ignoring %s...", ld.Title)
		}
	}
}

func (link *LinkDetails) ErrorOccurred(err error) {
	link.State = LinkTransientError
	link.LastError = err
}

// sort.Interface for ArticleArray.
func (aa ArticleArray) Len() int {
	return len(aa)
}

func (aa ArticleArray) Less(i, j int) bool {
	return aa[i].PubDate.Before(aa[j].PubDate)
}

func (aa ArticleArray) Swap(i, j int) {
	aa[i], aa[j] = aa[j], aa[i]
}
