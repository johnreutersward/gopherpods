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
	"google.golang.org/appengine/urlfetch"

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
	h      func(ctx context.Context, w http.ResponseWriter, r *http.Request) error
	method string
}

func (a aehandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != a.method {
		http.NotFound(w, r)
		return
	}

	ctx := appengine.NewContext(r)
	if err := a.h(ctx, w, r); err != nil {
		log.Errorf(ctx, "%v", err)
		http.Error(w, "There was an error, sorry", http.StatusInternalServerError)
		return
	}
}

func init() {
	http.Handle("/", aehandler{podcastsHandler, "GET"})
	http.Handle("/submit/", aehandler{submitHandler, "GET"})
	http.Handle("/submit/add", aehandler{submitAddHandler, "POST"})

	http.Handle("/submissions/", aehandler{submissionsHandler, "GET"})
	http.Handle("/submissions/add", aehandler{submissionsAddHandler, "POST"})
	http.Handle("/submissions/del", aehandler{submissionsDelHandler, "POST"})

	http.Handle("/tasks/email", aehandler{emailHandler, "GET"})
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

func sanitize(xss string) string {
	return template.HTMLEscapeString(template.JSEscapeString(strings.TrimSpace(xss)))
}

type RecaptchaResponse struct {
	Success   bool     `json:"success"`
	ErrorCode []string `json:"error-codes"`
}

func submitAddHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return err
	}

	form := url.Values{}
	form.Add("secret", os.Getenv("SECRET"))
	form.Add("response", r.FormValue("g-recaptcha-response"))
	form.Add("remoteip", r.RemoteAddr)
	req, err := http.NewRequest("POST", "https://www.google.com/recaptcha/api/siteverify", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	cli := urlfetch.Client(ctx)
	resp, err := cli.Do(req)
	if err != nil {
		return err
	}

	var recaptcha RecaptchaResponse
	if err := json.NewDecoder(resp.Body).Decode(&recaptcha); err != nil {
		return err
	}

	if !recaptcha.Success {
		log.Infof(ctx, "%v", recaptcha)
		return fmt.Errorf("reCAPTCHA check failed")
	}

	date, err := time.Parse(yyyymmdd, sanitize(r.FormValue("date")))
	if err != nil {
		return err
	}

	sub := Submission{
		Show:      sanitize(r.FormValue("show")),
		Title:     sanitize(r.FormValue("title")),
		Desc:      sanitize(r.FormValue("desc")),
		URL:       template.URL(sanitize(r.FormValue("url"))),
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

func emailHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	keys, err := datastore.NewQuery("Submission").KeysOnly().GetAll(ctx, nil)
	if err != nil {
		return err
	}

	if len(keys) == 0 {
		return nil
	}

	msg := mail.Message{
		Subject: "GopherPods",
		Sender:  os.Getenv("EMAIL"),
		Body: fmt.Sprintf("There are %d outstanding submissions.\n\nhttps://gopherpods.appspot.com/submissions/\n\n%v",
			len(keys), time.Now()),
	}

	if err := mail.SendToAdmins(ctx, &msg); err != nil {
		return err
	}

	return nil
}
