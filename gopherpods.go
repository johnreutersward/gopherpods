package gopherpods

import (
	"html/template"
	"net/http"
	"time"

	"google.golang.org/appengine"
	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/log"

	"github.com/gorilla/mux"
	"github.com/jgrahamc/bluemonday"
	"golang.org/x/net/context"
)

const (
	yyyymmdd = "2006-01-02"
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
)

type aehandler struct {
	h func(ctx context.Context, w http.ResponseWriter, r *http.Request) error
}

func (a aehandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)

	if err := a.h(ctx, w, r); err != nil {
		log.Errorf(ctx, "%v", err)
		http.Error(w, "There was an error, sorry", http.StatusInternalServerError)
		return
	}
}

func init() {
	r := mux.NewRouter()

	r.Handle("/", aehandler{podcastsHandler}).Methods("GET")
	r.Handle("/submit/", aehandler{submitHandler}).Methods("GET")
	r.Handle("/submit/add", aehandler{submitAddHandler}).Methods("POST")

	r.Handle("/submissions/", aehandler{submissionsHandler}).Methods("GET")
	r.Handle("/submissions/add", aehandler{submissionsAddHandler}).Methods("POST")
	r.Handle("/submissions/del", aehandler{submissionsDelHandler}).Methods("POST")

	http.Handle("/", r)
}

type Podcast struct {
	ID    int64        `datastore:",noindex"`
	Show  string       `datastore:",noindex"`
	Title string       `datastore:",noindex"`
	Desc  string       `datastore:",noindex"`
	URL   template.URL `datastore:",noindex"`
	Date  time.Time    `datastore:""`
	Added time.Time    `datastore:""`
}

func (p *Podcast) DateFormatted() string {
	return p.Date.Format(yyyymmdd)
}

type Submission struct {
	Show      string       `datastore:",noindex"`
	Title     string       `datastore:",noindex"`
	Desc      string       `datastore:",noindex"`
	URL       template.URL `datastore:",noindex"`
	Date      time.Time    `datastore:",noindex"`
	Submitted time.Time    `datastore:""`
	Key       string       `datastore:"-"`
}

func (s *Submission) DateFormatted() string {
	return s.Date.Format(yyyymmdd)
}

func podcastsHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	podcasts := make([]Podcast, 0)
	if _, err := datastore.NewQuery("Podcast").GetAll(ctx, &podcasts); err != nil {
		return err
	}

	var tmplData = struct {
		Podcasts []Podcast
	}{
		podcasts,
	}

	return podcastsTmpl.ExecuteTemplate(w, "base", tmplData)
}

func submitHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	return submitTmpl.ExecuteTemplate(w, "base", nil)
}

func submitAddHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return err
	}

	policy := bluemonday.StrictPolicy()

	date, err := time.Parse(yyyymmdd, policy.Sanitize(r.FormValue("date")))
	if err != nil {
		return err
	}

	sub := Submission{
		Show:      policy.Sanitize(r.FormValue("show")),
		Title:     policy.Sanitize(r.FormValue("title")),
		Desc:      policy.Sanitize(r.FormValue("desc")),
		URL:       template.URL(policy.Sanitize(r.FormValue("url"))),
		Submitted: time.Now(),
		Date:      date,
	}

	if _, err := datastore.Put(ctx, datastore.NewIncompleteKey(ctx, "Submission", nil), &sub); err != nil {
		return err
	}

	return thanksTmpl.ExecuteTemplate(w, "base", nil)
}

func submissionsHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	submissions := make([]Submission, 0)
	keys, err := datastore.NewQuery("Submission").Order("Submitted").GetAll(ctx, &submissions)
	if err != nil {
		return err
	}

	for i := range submissions {
		submissions[i].Key = keys[i].Encode()
	}

	var tmplData = struct {
		Submissions []Submission
	}{
		Submissions: submissions,
	}

	return submissionsTmpl.ExecuteTemplate(w, "base", tmplData)
}

func submissionsAddHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return err
	}

	ID, _, err := datastore.AllocateIDs(ctx, "Podcast", nil, 100)
	if err != nil {
		return err
	}

	date, err := time.Parse(yyyymmdd, r.FormValue("date"))
	if err != nil {
		return err
	}

	podcast := Podcast{
		ID:    ID,
		Show:  r.FormValue("show"),
		Title: r.FormValue("title"),
		Desc:  r.FormValue("desc"),
		URL:   template.URL(r.FormValue("url")),
		Date:  date,
		Added: time.Now(),
	}

	if _, err := datastore.Put(ctx, datastore.NewKey(ctx, "Podcast", "", ID, nil), &podcast); err != nil {
		return err
	}

	key, err := datastore.DecodeKey(r.FormValue("key"))
	if err != nil {
		return err
	}

	if err := datastore.Delete(ctx, key); err != nil {
		return err
	}

	return successTmpl.ExecuteTemplate(w, "base", nil)
}

func submissionsDelHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return err
	}

	key, err := datastore.DecodeKey(r.FormValue("key"))
	if err != nil {
		return err
	}

	if err := datastore.Delete(ctx, key); err != nil {
		return err
	}

	return successTmpl.ExecuteTemplate(w, "base", nil)
}
