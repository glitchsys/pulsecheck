package httpapi

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"

	"pulsecheck/internal/mail"
	"pulsecheck/internal/store"
	"pulsecheck/internal/web"
)

func fsSub() (fs.FS, error) {
	return fs.Sub(web.Files, "static")
}

func randRead(b []byte) (int, error) { return rand.Read(b) }

func timeNow() time.Time { return time.Now() }

type view struct {
	Title         string
	Product       string
	OfficialURL   string
	CSRF          string
	NoIndex       bool
	ShowLogout    bool
	Level         string
	LevelClass    string
	LastHour      int
	LastDay       int
	FilterSlug    string
	FilterName    string
	BucketMinutes int
	Empty         bool
	Stats         []statView
	Bars          []barView
	Rows          []rowView
	Services      []optView
	Categories    []optView
	Heading       string
	Message       string
	LoginError    string
	Reports       []reportView
}

type statView struct {
	Slug     string
	Name     string
	LastHour int
	LastDay  int
	Current  bool
}

type barView struct {
	X, Y, W, H string
	Level      string
	Label      string
	Count      int
}

type rowView struct {
	Label string
	Count int
}

type optView struct {
	ID, Label string
	Selected  bool
}

type reportView struct {
	ID          int64
	Component   string
	Category    string
	Description string
	Email       string
	EmailStatus string
	When        string
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimSpace(r.URL.Query().Get("service"))
	now := timeNow()
	dash, err := s.store.Dashboard(r.Context(), slug, now)
	if errors.Is(err, store.ErrUnknownService) {
		v := s.newView(w, r, "Unknown service")
		v.NoIndex = true
		v.Heading = "Unknown service"
		v.Message = "That service is not on this page."
		s.render(w, "result", http.StatusNotFound, v)
		return
	}
	if err != nil {
		s.log.Error("dashboard", "err", err)
		v := s.newView(w, r, "Unavailable")
		v.NoIndex = true
		v.Heading = "Page unavailable"
		v.Message = "The chart could not be loaded. Try again shortly."
		s.render(w, "result", http.StatusInternalServerError, v)
		return
	}

	v := s.newView(w, r, s.cfg.ProductName)
	v.FilterSlug = dash.FilterSlug
	v.FilterName = dash.FilterName
	if v.FilterName == "" {
		v.FilterName = "All services"
	}
	v.BucketMinutes = s.cfg.BucketMinutes
	for _, svc := range dash.Services {
		current := dash.FilterSlug != "" && svc.Slug == dash.FilterSlug
		v.Stats = append(v.Stats, statView{
			Slug: svc.Slug, Name: svc.Name, LastHour: svc.LastHour, LastDay: svc.LastDay, Current: current,
		})
		if dash.FilterSlug == "" {
			v.LastHour += svc.LastHour
			v.LastDay += svc.LastDay
		} else if current {
			v.LastHour = svc.LastHour
			v.LastDay = svc.LastDay
		}
		v.Services = append(v.Services, optView{ID: svc.Slug, Label: svc.Name, Selected: current || (dash.FilterSlug == "" && len(v.Services) == 0)})
	}
	if dash.FilterSlug == "" && len(v.Services) > 0 {
		v.Services[0].Selected = true
	}
	v.LevelClass, v.Level = store.SignalLevel(v.LastHour, s.cfg.SpikeAmber, s.cfg.SpikeRed)
	v.Empty = v.LastDay == 0
	v.Bars = layoutBars(dash.Points, s.cfg.SpikeAmber, s.cfg.SpikeRed)
	for _, bar := range v.Bars {
		v.Rows = append(v.Rows, rowView{Label: bar.Label, Count: bar.Count})
	}
	for _, c := range store.Categories() {
		v.Categories = append(v.Categories, optView{ID: c.ID, Label: c.Label})
	}
	s.render(w, "home", http.StatusOK, v)
}

func layoutBars(points []store.Point, amber, red int) []barView {
	const width = 960.0
	const height = 140.0
	n := len(points)
	if n == 0 {
		return nil
	}
	max := 0
	for _, p := range points {
		if p.Count > max {
			max = p.Count
		}
	}
	slot := width / float64(n)
	gap := slot * 0.25
	if gap > 3 {
		gap = 3
	}
	bw := slot - gap
	if bw < 1 {
		bw = 1
	}
	bars := make([]barView, 0, n)
	for i, p := range points {
		h := 0.0
		if p.Count > 0 && max > 0 {
			h = float64(p.Count) / float64(max) * (height - 8)
			if h < 3 {
				h = 3
			}
		}
		level := "empty"
		if p.Count > 0 {
			level, _ = store.SignalLevel(p.Count, amber, red)
		}
		bars = append(bars, barView{
			X:     fmt.Sprintf("%.1f", float64(i)*slot),
			Y:     fmt.Sprintf("%.1f", height-h),
			W:     fmt.Sprintf("%.1f", bw),
			H:     fmt.Sprintf("%.1f", h),
			Level: level,
			Label: time.Unix(p.Start, 0).UTC().Format("2006-01-02 15:04 UTC"),
			Count: p.Count,
		})
	}
	return bars
}

func (s *Server) report(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, http.StatusBadRequest, "The form could not be read.")
		return
	}
	if !s.csrfOK(r) {
		s.fail(w, r, http.StatusBadRequest, "The form expired. Reload the page and try again.")
		return
	}
	if strings.TrimSpace(r.FormValue("company")) != "" {
		s.log.Info("dropped honeypot submission")
		s.recorded(w, r)
		return
	}

	sub := store.Submission{
		Slug:         r.FormValue("service"),
		Category:     r.FormValue("category"),
		Description:  r.FormValue("description"),
		Email:        r.FormValue("email"),
		IPHash:       hashIP(s.cfg.IPHashPepper, clientIP(r, s.cfg.TrustProxy)),
		Now:          timeNow(),
		DedupeWindow: s.cfg.DedupeWindow,
		EmailStatus:  store.EmailSkipped,
	}
	if s.mail.Enabled() {
		sub.EmailStatus = store.EmailPending
	}
	result, err := s.store.Submit(r.Context(), sub)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrUnknownService):
			s.fail(w, r, http.StatusBadRequest, "Choose a service from the list.")
		case errors.Is(err, store.ErrBadCategory):
			s.fail(w, r, http.StatusBadRequest, "Choose a problem type from the list.")
		case errors.Is(err, store.ErrDescription):
			s.fail(w, r, http.StatusBadRequest, "The description is limited to 2,000 characters.")
		case errors.Is(err, store.ErrEmail):
			s.fail(w, r, http.StatusBadRequest, "The email address does not look valid.")
		default:
			s.log.Error("submit report", "err", err)
			s.fail(w, r, http.StatusInternalServerError, "Something went wrong. Try again.")
		}
		return
	}
	if result.Duplicate {
		v := s.newView(w, r, "Already reported")
		v.NoIndex = true
		v.Heading = "Already reported"
		v.Message = "You've already reported an issue for this service recently. Your earlier signal is still counted."
		s.render(w, "result", http.StatusTooManyRequests, v)
		return
	}
	if s.mail.Enabled() {
		s.deliver(r, result)
	}
	s.recorded(w, r)
}

func (s *Server) deliver(r *http.Request, result store.Submitted) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 8*time.Second)
	defer cancel()
	err := s.mail.Send(ctx, mail.Notice{
		ID:            result.ID,
		Product:       s.cfg.ProductName,
		Component:     result.ComponentName,
		Category:      store.CategoryLabel(result.Category),
		Description:   result.Description,
		ReporterEmail: result.Email,
		When:          result.CreatedAt,
		AdminURL:      s.baseURL(r) + "/admin",
	})
	status := store.EmailSent
	errMsg := ""
	if err != nil {
		status = store.EmailFailed
		errMsg = err.Error()
		s.log.Warn("support email failed", "report_id", result.ID, "err", err)
	}
	if uerr := s.store.SetEmailStatus(context.WithoutCancel(r.Context()), result.ID, status, errMsg); uerr != nil {
		s.log.Error("email status", "report_id", result.ID, "err", uerr)
	}
}

func (s *Server) recorded(w http.ResponseWriter, r *http.Request) {
	v := s.newView(w, r, "Report recorded")
	v.NoIndex = true
	v.Heading = "Report recorded"
	v.Message = "Your report has been recorded and counted on the chart. If you included a description or email, the operator receives it privately. This is not confirmation of an outage."
	s.render(w, "result", http.StatusOK, v)
}

func (s *Server) fail(w http.ResponseWriter, r *http.Request, status int, message string) {
	v := s.newView(w, r, "Report not saved")
	v.NoIndex = true
	v.Heading = "That report was not saved"
	v.Message = message
	s.render(w, "result", status, v)
}

func (s *Server) loginForm(w http.ResponseWriter, r *http.Request) {
	if s.adminOK(r) {
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}
	v := s.newView(w, r, "Admin login")
	v.NoIndex = true
	s.render(w, "login", http.StatusOK, v)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	if err := r.ParseForm(); err != nil {
		s.loginError(w, r, http.StatusBadRequest, "The form could not be read.")
		return
	}
	if !s.csrfOK(r) {
		s.loginError(w, r, http.StatusBadRequest, "The form expired. Reload the page and try again.")
		return
	}
	ip := hashIP(s.cfg.IPHashPepper, clientIP(r, s.cfg.TrustProxy))
	allowed, err := s.store.AllowLogin(r.Context(), ip, timeNow())
	if err != nil {
		s.log.Error("login limit", "err", err)
		s.loginError(w, r, http.StatusInternalServerError, "Something went wrong. Try again.")
		return
	}
	if !allowed {
		s.loginError(w, r, http.StatusTooManyRequests, "Too many attempts. Try again later.")
		return
	}
	if !passwordOK(r.FormValue("password"), s.cfg.AdminPassword) {
		time.Sleep(200 * time.Millisecond)
		s.loginError(w, r, http.StatusUnauthorized, "Invalid password.")
		return
	}
	exp := timeNow().Add(12 * time.Hour).Unix()
	http.SetCookie(w, s.cookie(adminCookie, sign(s.cfg.SessionSecret, strconv.FormatInt(exp, 10)), 12*60*60))
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (s *Server) loginError(w http.ResponseWriter, r *http.Request, status int, message string) {
	v := s.newView(w, r, "Admin login")
	v.NoIndex = true
	v.LoginError = message
	s.render(w, "login", status, v)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || !s.csrfOK(r) {
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
		return
	}
	http.SetCookie(w, s.cookie(adminCookie, "", -1))
	http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
}

func (s *Server) admin(w http.ResponseWriter, r *http.Request) {
	if !s.adminOK(r) {
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
		return
	}
	reports, err := s.store.ListReports(r.Context(), 100)
	if err != nil {
		s.log.Error("list reports", "err", err)
		v := s.newView(w, r, "Admin")
		v.NoIndex = true
		v.ShowLogout = true
		v.Heading = "Reports unavailable"
		v.Message = "The report list could not be loaded."
		s.render(w, "result", http.StatusInternalServerError, v)
		return
	}
	v := s.newView(w, r, "Reports")
	v.NoIndex = true
	v.ShowLogout = true
	for _, rep := range reports {
		email := rep.Email
		if email == "" {
			email = "—"
		}
		desc := rep.Description
		if desc == "" {
			desc = "—"
		}
		v.Reports = append(v.Reports, reportView{
			ID:          rep.ID,
			Component:   rep.Component,
			Category:    store.CategoryLabel(rep.Category),
			Description: desc,
			Email:       email,
			EmailStatus: rep.EmailStatus,
			When:        rep.CreatedAt.Format("2006-01-02 15:04 UTC"),
		})
	}
	s.render(w, "admin", http.StatusOK, v)
}

func (s *Server) deleteReport(w http.ResponseWriter, r *http.Request) {
	if !s.adminOK(r) {
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil || !s.csrfOK(r) {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		http.NotFound(w, r)
		return
	}
	if err := s.store.DeleteReport(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		s.log.Error("delete report", "id", id, "err", err)
		http.Error(w, "could not delete report", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}
