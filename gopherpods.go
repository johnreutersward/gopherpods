package main

import (
	"encoding/json"
	"flag"
	"html/template"
	"log"
	"os"
	"sort"
	"time"

	"github.com/SlyMarbo/rss"
	"github.com/gorilla/feeds"
	"github.com/jaytaylor/html2text"
)

const (
	dateLayout        = `"2006-01-02"`
	displayDateLayout = "02 Jan 2006"
)

var tmpl = template.Must(template.ParseFiles("index.gohtml"))

type episode struct {
	Show  string `json:"show"`
	Title string `json:"title"`
	About string `json:"about"`
	URL   string `json:"url"`
	Date  date   `json:"date"`
}

func (e *episode) GetDisplayDate() string { return (time.Time)(e.Date).Format(displayDateLayout) }

type date time.Time

func (d date) Before(t date) bool            { return (time.Time)(d).Before(time.Time(t)) }
func (d date) Format() string                { return (time.Time)(d).Format(dateLayout) }
func (d *date) MarshalJSON() ([]byte, error) { return []byte(d.Format()), nil }
func (d *date) UnmarshalJSON(data []byte) error {
	t, err := time.Parse(dateLayout, string(data))
	*d = date(t)
	return err
}

type episodes []episode

func (e episodes) Len() int               { return len(e) }
func (e episodes) Less(i int, j int) bool { return e[i].Date.Before(e[j].Date) }
func (e episodes) Swap(i int, j int)      { e[i], e[j] = e[j], e[i] }

func main() {
	feedUrl := flag.String("feed", "", "Get new episodes from feed URL and add to episodes.json.")
	feedStartUrl := flag.String("start-url", "", "Start from episode with this url when using -feed.")
	flag.Parse()

	if *feedUrl == "" {
		episodes := parseEpisodes()
		sort.Sort(sort.Reverse(episodes))
		createSite(episodes)
		createFeeds(episodes)
	} else {
		newEpisodes := getEpisodesFromFeed(*feedUrl, *feedStartUrl)
		episodes := parseEpisodes()
		episodes = append(episodes, newEpisodes...)
		sort.Sort(episodes)
		writeFeed(episodes)
	}
}

func parseEpisodes() episodes {
	episodesFile, err := os.Open("episodes.json")
	if err != nil {
		log.Fatal(err)
	}
	defer episodesFile.Close()

	var episodes episodes
	if err := json.NewDecoder(episodesFile).Decode(&episodes); err != nil {
		log.Fatal(err)
	}

	return episodes
}

func createSite(episodes episodes) {
	indexFile := createFile("static/index.html")
	defer indexFile.Close()

	if err := tmpl.ExecuteTemplate(indexFile, "index", episodes); err != nil {
		log.Fatal(err)
	}
}

func createFeeds(episodes episodes) {
	rssFile := createFile("static/rss.xml")
	defer rssFile.Close()

	atomFile := createFile("static/atom.xml")
	defer atomFile.Close()

	feed := &feeds.Feed{
		Title:       "GopherPods",
		Link:        &feeds.Link{Href: "https://gopherpods.netlify.com"},
		Description: "GopherPods is a community-driven list of podcast episodes that cover the Go programming language and Go related projects.",
		Author:      &feeds.Author{Name: "John Reuterswärd", Email: "john.reutersward@gmail.com"},
	}

	for _, episode := range episodes {
		item := &feeds.Item{
			Title:       episode.Title,
			Description: episode.About,
			Link:        &feeds.Link{Href: episode.URL},
			Author:      &feeds.Author{Name: episode.Show},
			Created:     time.Time(episode.Date),
		}
		feed.Add(item)
	}

	feed.Created = feed.Items[0].Created

	if err := feed.WriteRss(rssFile); err != nil {
		log.Fatal(err)
	}

	if err := feed.WriteAtom(atomFile); err != nil {
		log.Fatal(err)
	}
}

func getEpisodesFromFeed(feedUrl string, startUrl string) episodes {
	feed, err := rss.Fetch(feedUrl)
	if err != nil {
		log.Fatal(err)
	}

	var newEpisodes episodes
	for i := range feed.Items {
		about, err := html2text.FromString(feed.Items[i].Summary, html2text.Options{OmitLinks: true})
		if err != nil {
			log.Fatal(err)
		}

		newEpisode := episode{
			Show:  feed.Title,
			Title: feed.Items[i].Title,
			About: about,
			URL:   feed.Items[i].Link,
			Date:  date(feed.Items[i].Date),
		}
		newEpisodes = append(newEpisodes, newEpisode)
	}

	var episodes episodes

	if startUrl != "" {
		sort.Sort(newEpisodes)
		var foundStart bool
		for i := range newEpisodes {
			if !foundStart {
				if startUrl == newEpisodes[i].URL {
					foundStart = true
				} else {
					continue
				}
			}
			episodes = append(episodes, newEpisodes[i])
		}
	} else {
		episodes = newEpisodes
	}

	return episodes
}

func writeFeed(episodes episodes) {
	episodesFile := createFile("episodes.json")
	defer episodesFile.Close()

	enc := json.NewEncoder(episodesFile)
	enc.SetIndent("", "	")
	if err := enc.Encode(episodes); err != nil {
		log.Fatal(err)
	}
}

func createFile(filePath string) *os.File {
	file, err := os.Create(filePath)
	if err != nil {
		log.Fatal(err)
	}
	return file
}
