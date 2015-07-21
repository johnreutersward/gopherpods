package gopherpods

import (
	"html/template"
	"net/http"

	"google.golang.org/appengine"
	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/log"

	"golang.org/x/net/context"
)

func init() {
	http.Handle("/", aehandler{podcastsHandler}).Methods("GET")
	http.Handle("/feed", aehandler{feedHandler}).Methods("GET")
	http.Handle("/submit", aehandler{submitHandler}).Methods("GET")
	http.Handle("/submit/add", aehandler{submitAddHandler}).Methods("POST")

	http.Handle("/submissions", aehandler{submissionsHandler}).Methods("GET")
	http.Handle("/submissions/add", aehandler{submissionsAddHandler}).Methods("POST")
	http.Handle("/submissions/del", aehandler{submissionsDelHandler}).Methods("DELETE")
}

var (
	podcastsTmpl = template.Must(template.ParseFiles(
		"static/html/base.html",
		"static/html/podcasts.html",
	))

	submitTmpl = template.Must(template.ParseFiles(
		"static/html/base.html",
		"static/html/submit.html",
	))

	submissionsTmpl = template.Must(template.ParseFiles(
		"static/html/base.html",
		"static/html/submissions.html",
	))
)

type aehandler struct {
	h func(ctx context.Context, w http.ResponseWriter, r *http.Request) error
}

func (a aehandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)

	if err := a.h(ctx, w, r); err != nil {
		log.Errorf(ctx, "%v", err)
		http.Error(w, "Something went wrong, sorry", http.StatusInternalServerError)
		return
	}

}

type Podcast struct {
	ID    int64
	Show  string
	Title string
	Desc  string
	URL   string
	Date  time.Time
	Added time.Time
}

type Submission struct {
	Show      string
	Title     string
	Desc      string
	URL       string
	Date      time.Time
	Submitted time.Time
}

func podcastsHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	keys, err := datastore.NewQuery("Podcast").KeysOnly().GetAll(ctx, nil)
	if err != nil {
		return err
	}

	podcasts := make([]Podcast, 0, len(keys))
	if err := datastore.GetMulti(ctx, keys, podcasts); err != nil {
		return err
	}

	var tmplData = struct {
		Podcasts []Podcast
	}{
		Podcast: podcasts,
	}

	return podcastsTmpl.Execute(w, tmplData)
}

func feedHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) error        { return nil }
func submitHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) error      { return nil }
func submitAddHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) error   { return nil }
func submissionsHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) error { return nil }
func submissionsAddHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	return nil
}
func submissionsDelHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	return nil
}
