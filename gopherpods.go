package main

import (
	"encoding/json"
	"html/template"
	"log"
	"os"
	"sort"
	"time"

	"github.com/gorilla/feeds"
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

func (e *episode) GetDisplayDate() string {
	return (time.Time)(e.Date).Format(displayDateLayout)
}

type date time.Time

func (d *date) UnmarshalJSON(data []byte) error {
	t, err := time.Parse(dateLayout, string(data))
	*d = date(t)
	return err
}

func (d date) Before(t date) bool {
	return (time.Time)(d).Before(time.Time(t))
}

type episodes []episode

func (e episodes) Len() int               { return len(e) }
func (e episodes) Less(i int, j int) bool { return e[i].Date.Before(e[j].Date) }
func (e episodes) Swap(i int, j int)      { e[i], e[j] = e[j], e[i] }

func main() {
	episodes := parseEpisodes()
	sort.Sort(sort.Reverse(episodes))
	createSite(episodes)
	createFeeds(episodes)
}

func parseEpisodes() episodes {
	jsonFile, err := os.Open("episodes.json")
	if err != nil {
		log.Fatal(err)
	}

	var episodes episodes
	if err := json.NewDecoder(jsonFile).Decode(&episodes); err != nil {
		log.Fatal(err)
	}

	return episodes
}

func createSite(episodes episodes) {
	indexFile := createFile("index.html")
	if err := tmpl.ExecuteTemplate(indexFile, "index", episodes); err != nil {
		log.Fatal(err)
	}
}

func createFile(fileName string) *os.File {
	file, err := os.Create(fileName)
	if err != nil {
		log.Fatal(err)
	}
	return file
}

func createFeeds(episodes episodes) {
	rssFile := createFile("rss.xml")
	atomFile := createFile("atom.xml")

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
