package main

import (
	"bytes"
	"encoding/xml"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type RSSHandler struct {
	Feeds []RSSFeed
}

type Cache interface {
	Seen(guid string) bool
	Add(guid string) error
	Remove(guid string) error
}

type RamCache struct {
	// the bool means nothing. if this was C++, i'd use void
	cache map[string]bool
}

func NewRamCache() *RamCache {
	return &RamCache{
		cache: make(map[string]bool),
	}
}

func (rc *RamCache) Seen(guid string) bool {
	_, ok := rc.cache[guid]
	return ok
}

func (rc *RamCache) Add(guid string) error {
	rc.cache[guid] = true
	return nil
}

func (rc *RamCache) Remove(guid string) error {
	delete(rc.cache, guid)
	return nil
}

type RSSFeed struct {
	url   string
	cache Cache
	Rx    chan *FeedItem
}

type FeedItem struct {
	Title       string `xml:"title"`
	Description string `xml:"description"`
	Link        string `xml:"link"`
	Guid        string `xml:"guid"`
}

type Feed struct {
	Channel struct {
		Title       string      `xml:"title"`
		Link        string      `xml:"link"`
		Description string      `xml:"description"`
		Language    string      `xml:"language"`
		Items       []*FeedItem `xml:"item"`
	} `xml:"channel"`
}

func NewRSSFeed(url string, cache Cache) *RSSFeed {
	f := &RSSFeed{
		url:   url,
		cache: cache,
		Rx:    make(chan *FeedItem),
	}
	go f.worker()
	return f
}

func (rf *RSSFeed) worker() {
	firstrun := true
	for {
		var contents []byte
		var f *Feed
		var body io.Reader
		var err error

		log.Printf("Fetching URL: %s", rf.url)
		if strings.HasPrefix(rf.url, "file://") {
			filePath := strings.TrimPrefix(rf.url, "file://")
			body, err = os.Open(filePath)
		} else {
			var res *http.Response
			res, err = http.Get(rf.url)
			body = io.Reader(res.Body)
		}

		if err != nil {
			log.Printf("Error getting url '%s': %s", rf.url, err)
			goto refetch
		}

		contents, err = ioutil.ReadAll(body)
		if err != nil {
			log.Printf("Error reading page contents for '%s': %s", rf.url, err)
			goto refetch
		}

		if string(contents[:4]) == "<?xml" {
			contents = bytes.SplitN(contents, []byte{'\n'}, 2)[1]
		}

		//log.Printf("RAW XML: %s: ENDXML", contents)
		f = &Feed{}
		err = xml.Unmarshal(contents, f)
		if err != nil {
			log.Printf("Error parsing XML for '%s': %s", rf.url, err)
			goto refetch
		}

		//log.Printf("GOT XML: %s: XML", f.Channel)
		for _, e := range f.Channel.Items {
			id := e.Guid
			if len(id) == 0 {
				id = e.Title
			}

			log.Printf("Checking if cache has seen guid: %s", id)
			if rf.cache.Seen(id) {
				continue
			}

			log.Printf("ADDING GUID: %s", id)
			rf.cache.Add(id)
			if !firstrun {
				rf.Rx <- e
			}
		}
		firstrun = false

	refetch:
		re := time.Minute * Conf.Refetch
		select {
		case <-time.After(re):
			log.Printf(re.String()+" elapsed. refetching URL: %s", rf.url)
		}
	}
}
