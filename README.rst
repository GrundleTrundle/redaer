==========
What is it
==========

I wanted a tiny project I could write to get used to programming 
in Go. So I wrote a simple feed aggregator.

Basic Operation
===============

The idea is that you run *redaer* with a HTML file of links
as its input. (typically, for me this is a bookmark.html file).
For each of the links in the input file, a summary of all new unread articles
is generated as HTML to stdout.  When a feed has been successfully summarized, 
it's marked as read. 

All information related to a feed is marshalled out to a $HOME/redaer.json file.
I used JSON here, because there's no UI to speak of, so it's to edit the JSON file
to mark a link as *Ignore* (because not all links in my bookmarks have a RSS or atom feed.)

Typical invocation:
        redaer < ~/.w3m/bookmark.html > /tmp/summary.html

Since it's Go, it does check for updates concurrently - 8 concurrent goroutines at the moment.
Surprisingly clean to do with just channels and routines.

Limitations
===========

- It's only built for my simplistic workflow - run it once in the morning, and check out the new articles, 
  without having to click through N pages manually.

- It ignores content summaries. I just want the links.

- No timeout on the concurrent feed updates, so it can still take a while if
  just one site is having really slow responses.
