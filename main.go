package main

import (
	"fmt"
	"log"
	"os"
	"os/user"
	"path"
	"redaer/links"
	"redaer/store"
)

func main() {
	feedFile, err := UserFeedFile()
	if err != nil {
		log.Fatal("Couldn't locate home directory: %s", err)
	}

	str, err := store.Load(feedFile)
	if err != nil {
		log.Fatalf("Could not load %s: %s", feedFile, err)
	}

	lnks, err := links.Extract(os.Stdin)
	if err != nil {
		log.Fatal("Could not read links: ", err)
	}

	for _, lnk := range lnks {
		str.InterestedIn(lnk.Url, lnk.Title)
	}
	str.CheckForUpdates()

	for _, lnk := range lnks {
		details := str.Links[lnk.Url]
		if details.State != store.LinkHasFeed {
			if details.LastError != nil {
				fmt.Printf("<h1>%s</h1>\n", details.Title)
				fmt.Printf("<i>ERROR: %s</i>\n", details.LastError)
			}
			continue
		}

		unread := details.UnreadArticles()
		if len(unread) == 0 {
			continue
		}

		fmt.Printf("<h1>%s</h1>\n", details.Title)
		fmt.Printf("<ul>\n")
		// Feeds are in oldest first order.
		for _, art := range unread {
			fmt.Printf("<li><a href='%s'>%s</a>\n", art.Url, art.Title)
		}
		fmt.Printf("</ul>\n")
		details.MarkAllAsRead()
	}

	str.Save(feedFile)
}

func UserFeedFile() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", err
	}

	return path.Join(u.HomeDir, "redaer.json"), nil
}
