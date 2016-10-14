package gopherpods

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"strconv"
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
	http.HandleFunc("/feed", provideFeedHandler(providePodcastURL))
	http.HandleFunc("/podcast/feed", provideFeedHandler(providePodcastMediaURL))
	http.HandleFunc("/submissions", submissionsHandler)
	http.HandleFunc("/submissions/add", submissionsAddHandler)
	http.HandleFunc("/submissions/del", submissionsDelHandler)
	http.HandleFunc("/tasks/email", emailHandler)
}

type Podcast struct {
	ID       int64        `datastore:",noindex"`
	Show     string       `datastore:",noindex"`
	Title    string       `datastore:",noindex"`
	Desc     string       `datastore:",noindex"`
	URL      template.URL `datastore:",noindex"`
	MediaURL template.URL `datastore:",noindex"`
	Date     time.Time    `datastore:""`
	Added    time.Time    `datastore:""`
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

func providePodcastURL(p Podcast) string {
	return string(p.URL)
}

func providePodcastMediaURL(p Podcast) string {
	return string(p.MediaURL)
}

func provideFeedHandler(provideHref func(p Podcast) string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := appengine.NewContext(r)
		podcasts, err := getPodcasts(ctx)
		if err != nil {
			serveErr(ctx, err, w)
			return
		}

		feed := &feeds.Feed{
			Title:       "GopherPods",
			Link:        &feeds.Link{Href: "https://gopherpods.appspot.com"},
			Description: "Podcasts about Go (golang)",
			Created:     time.Now(),
		}

		for i := range podcasts {
			item := &feeds.Item{
				Title:       podcasts[i].Show + " - " + podcasts[i].Title,
				Link:        &feeds.Link{Href: provideHref(podcasts[i])},
				Description: podcasts[i].Desc,
				Id:          strconv.FormatInt(podcasts[i].ID, 10),
				Created:     podcasts[i].Date,
			}
			feed.Add(item)
		}

		w.Header().Set("Content-Type", "application/xml")
		if err := feed.WriteRss(w); err != nil {
			serveErr(ctx, err, w)
			return
		}
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
		ID:       ID,
		Show:     r.FormValue("show"),
		Title:    r.FormValue("title"),
		Desc:     r.FormValue("desc"),
		URL:      template.URL(r.FormValue("url")),
		MediaURL: template.URL(r.FormValue("media_url")),
		Date:     date,
		Added:    time.Now(),
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
