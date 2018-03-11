package gopherpods

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"google.golang.org/appengine"
	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/log"
	"google.golang.org/appengine/mail"
	"google.golang.org/appengine/memcache"
	"google.golang.org/appengine/urlfetch"

	"github.com/gorilla/feeds"
	"golang.org/x/net/context"
)

const (
	yyyymmdd     = "2006-01-02"
	displayDate  = "02 Jan 2006"
	recaptchaURL = "https://www.google.com/recaptcha/api/siteverify"
	cacheKey     = "podcasts"

	feedName        = "GopherPods"
	feedDescription = "Podcasts about Go (golang)"
	feedURL         = "https://gopherpods.appspot.com"
)

var (
	podcastsTmpl = template.Must(template.ParseFiles(
		"static/html/base.html",
		"static/html/podcasts.html",
	))

	submitTmpl = template.Must(template.ParseFiles(
		"static/html/base.html",
		"static/html/submit.html",
	))

	failedTmpl = template.Must(template.ParseFiles(
		"static/html/base.html",
		"static/html/failed.html",
	))

	thanksTmpl = template.Must(template.ParseFiles(
		"static/html/base.html",
		"static/html/thanks.html",
	))

	submissionsTmpl = template.Must(template.ParseFiles(
		"static/html/base.html",
		"static/html/submissions.html",
	))

	successTmpl = template.Must(template.ParseFiles(
		"static/html/base.html",
		"static/html/success.html",
	))

	errorTmpl = template.Must(template.ParseFiles(
		"static/html/base.html",
		"static/html/error.html",
	))
)

func serveErr(ctx context.Context, err error, w http.ResponseWriter) {
	log.Errorf(ctx, "%v", err)
	errorTmpl.ExecuteTemplate(w, "base", nil)
}

func init() {
	http.HandleFunc("/", podcastsHandler)
	http.HandleFunc("/submit", submitHandler)
	http.HandleFunc("/submit/add", submitAddHandler)
	http.HandleFunc("/feed", feedHandler)
	http.HandleFunc("/submissions", submissionsHandler)
	http.HandleFunc("/submissions/add", submissionsAddHandler)
	http.HandleFunc("/submissions/del", submissionsDelHandler)
	http.HandleFunc("/tasks/email", emailHandler)
	http.HandleFunc("/dump", jsonDumpHandler)
}

type Podcast struct {
	ID         int64        `datastore:",noindex" json:"-"`
	Show       string       `datastore:",noindex" json:"show"`
	Title      string       `datastore:",noindex" json:"title"`
	Desc       string       `datastore:",noindex" json:"about"`
	URL        template.URL `datastore:",noindex" json:"url"`
	MediaURL   template.URL `datastore:",noindex" json:"-"`
	RuntimeSec string       `datastore:",noindex" json:"-"`
	Size       string       `datastore:",noindex" json:"-"`
	Date       time.Time    `datastore:"" json:"date"`
	Added      time.Time    `datastore:"" json:"added"`
}

func (p *Podcast) DateFormatted() string {
	return p.Date.Format(displayDate)
}

type Submission struct {
	URL       template.URL `datastore:",noindex"`
	Submitted time.Time    `datastore:""`
	Key       string       `datastore:"-"`
}

func getPodcasts(ctx context.Context) ([]Podcast, error) {
	podcasts := make([]Podcast, 0)
	_, err := memcache.Gob.Get(ctx, cacheKey, &podcasts)
	if err != nil && err != memcache.ErrCacheMiss {
		log.Errorf(ctx, "memcache get error %v", err)
	}

	if err == nil {
		return podcasts, err
	}

	if _, err := datastore.NewQuery("Podcast").Order("-Date").GetAll(ctx, &podcasts); err != nil {
		return nil, err
	}

	if err := memcache.Gob.Set(ctx, &memcache.Item{Key: cacheKey, Object: &podcasts}); err != nil {
		log.Errorf(ctx, "memcache set error %v", err)
	}

	return podcasts, nil
}

func jsonDumpHandler(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)
	pods := make([]Podcast, 0)
	if _, err := datastore.NewQuery("Podcast").Order("Date").GetAll(ctx, &pods); err != nil {
		serveErr(ctx, err, w)
		return
	}
	json.NewEncoder(w).Encode(pods)
}

func podcastsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)
	podcasts, err := getPodcasts(ctx)
	if err != nil {
		serveErr(ctx, err, w)
		return
	}

	var tmplData = struct {
		Podcasts []Podcast
	}{
		podcasts,
	}

	podcastsTmpl.ExecuteTemplate(w, "base", tmplData)
}

func submitHandler(w http.ResponseWriter, r *http.Request) {
	submitTmpl.ExecuteTemplate(w, "base", nil)
}

type recaptchaResponse struct {
	Success   bool     `json:"success"`
	ErrorCode []string `json:"error-codes"`
}

func recaptchaCheck(ctx context.Context, response, ip string) (bool, error) {
	if appengine.IsDevAppServer() {
		return true, nil
	}

	form := url.Values{}
	form.Add("secret", os.Getenv("SECRET"))
	form.Add("response", response)
	form.Add("remoteip", ip)
	req, err := http.NewRequest("POST", recaptchaURL, strings.NewReader(form.Encode()))
	if err != nil {
		return false, err
	}

	cli := urlfetch.Client(ctx)

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	resp, err := cli.Do(req)
	if err != nil {
		return false, err
	}

	var recaptcha recaptchaResponse
	if err := json.NewDecoder(resp.Body).Decode(&recaptcha); err != nil {
		return false, err
	}

	if !recaptcha.Success {
		log.Warningf(ctx, "%+v", recaptcha)
		return false, nil
	}

	return true, nil
}

func submitAddHandler(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)
	if err := r.ParseForm(); err != nil {
		serveErr(ctx, err, w)
		return
	}

	success, err := recaptchaCheck(ctx, r.FormValue("g-recaptcha-response"), r.RemoteAddr)
	if err != nil {
		serveErr(ctx, err, w)
		return
	}

	if !success {
		log.Warningf(ctx, "reCAPTCHA check failed")
		failedTmpl.ExecuteTemplate(w, "base", nil)
		return
	}

	sub := Submission{
		URL:       template.URL(strings.TrimSpace(r.FormValue("url"))),
		Submitted: time.Now(),
	}

	if _, err := datastore.Put(ctx, datastore.NewIncompleteKey(ctx, "Submission", nil), &sub); err != nil {
		serveErr(ctx, err, w)
		return
	}

	thanksTmpl.ExecuteTemplate(w, "base", nil)
}

func feedHandler(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)
	podcasts, err := getPodcasts(ctx)
	if err != nil {
		serveErr(ctx, err, w)
		return
	}

	feed := &feeds.Feed{
		Title:       feedName,
		Link:        &feeds.Link{Href: feedURL},
		Description: feedDescription,
		Updated:     time.Now(),
	}

	for _, pod := range podcasts {
		feed.Add(&feeds.Item{
			Title:       pod.Show + " - " + pod.Title,
			Description: pod.Desc,
			Id:          string(pod.URL),
			Link: &feeds.Link{
				Href:   string(pod.MediaURL),
				Length: pod.Size,
				Type:   "audio/mpeg",
			},
			Created: pod.Date,
		})
	}

	rss := &feeds.Rss{feed}
	rssFeed := rss.RssFeed()

	rssFeed.Image = &feeds.RssImage{
		Title: feedName,
		Link:  feedURL,
		Url:   "https://gopherpods.appspot.com/img/gopher.png",
	}

	for i := 0; i < len(rssFeed.Items); i++ {
		rssFeed.Items[i].Link = rssFeed.Items[i].Guid
	}

	w.Header().Set("Content-Type", "application/xml")
	if err := feeds.WriteXML(rssFeed, w); err != nil {
		serveErr(ctx, err, w)
		return
	}
}

func submissionsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)
	submissions := make([]Submission, 0)
	keys, err := datastore.NewQuery("Submission").Order("Submitted").GetAll(ctx, &submissions)
	if err != nil {
		serveErr(ctx, err, w)
		return
	}

	for i := range submissions {
		submissions[i].Key = keys[i].Encode()
	}

	var tmplData = struct {
		Submissions []Submission
	}{
		Submissions: submissions,
	}

	submissionsTmpl.ExecuteTemplate(w, "base", tmplData)
}

func submissionsAddHandler(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)
	if err := r.ParseForm(); err != nil {
		serveErr(ctx, err, w)
		return
	}

	ID, _, err := datastore.AllocateIDs(ctx, "Podcast", nil, 1)
	if err != nil {
		serveErr(ctx, err, w)
		return
	}

	date, err := time.Parse(yyyymmdd, r.FormValue("date"))
	if err != nil {
		serveErr(ctx, err, w)
		return
	}

	podcast := Podcast{
		ID:         ID,
		Show:       r.FormValue("show"),
		Title:      r.FormValue("title"),
		Desc:       r.FormValue("desc"),
		URL:        template.URL(r.FormValue("url")),
		MediaURL:   template.URL(r.FormValue("media_url")),
		RuntimeSec: r.FormValue("runtime"),
		Size:       r.FormValue("size"),
		Date:       date,
		Added:      time.Now(),
	}

	if _, err := datastore.Put(ctx, datastore.NewKey(ctx, "Podcast", "", ID, nil), &podcast); err != nil {
		serveErr(ctx, err, w)
		return
	}

	key, err := datastore.DecodeKey(r.FormValue("key"))
	if err != nil {
		serveErr(ctx, err, w)
		return
	}

	if err := datastore.Delete(ctx, key); err != nil {
		serveErr(ctx, err, w)
		return
	}

	if err := memcache.Delete(ctx, cacheKey); err != nil {
		log.Errorf(ctx, "memcache delete error %v", err)
	}

	successTmpl.ExecuteTemplate(w, "base", nil)
}

func submissionsDelHandler(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)
	if err := r.ParseForm(); err != nil {
		serveErr(ctx, err, w)
		return
	}

	key, err := datastore.DecodeKey(r.FormValue("key"))
	if err != nil {
		serveErr(ctx, err, w)
		return
	}

	if err := datastore.Delete(ctx, key); err != nil {
		serveErr(ctx, err, w)
		return
	}

	successTmpl.ExecuteTemplate(w, "base", nil)
}

func emailHandler(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)
	keys, err := datastore.NewQuery("Submission").KeysOnly().GetAll(ctx, nil)
	if err != nil {
		serveErr(ctx, err, w)
		return
	}

	if len(keys) == 0 {
		return
	}

	msg := mail.Message{
		Subject: "GopherPods",
		Sender:  os.Getenv("EMAIL"),
		Body:    fmt.Sprintf("There are %d submissions", len(keys)),
	}

	if err := mail.SendToAdmins(ctx, &msg); err != nil {
		serveErr(ctx, err, w)
		return
	}
}
