package httpapi_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pulsecheck/internal/config"
	"pulsecheck/internal/httpapi"
	"pulsecheck/internal/mail"
	"pulsecheck/internal/store"
)

type fakeMail struct {
	err  error
	sent int
}

func (f *fakeMail) Enabled() bool { return true }

func (f *fakeMail) Send(context.Context, mail.Notice) error {
	f.sent++
	return f.err
}

func newApp(t *testing.T, m mail.Mailer) http.Handler {
	t.Helper()
	cfg := config.Config{
		Driver:        "sqlite",
		SQLitePath:    filepath.Join(t.TempDir(), "pulsecheck.db"),
		AdminPassword: "test-password-123",
		SessionSecret: "session-secret-value",
		IPHashPepper:  "ip-hash-pepper-value",
		ProductName:   "PulseCheck",
		Services: []config.Service{
			{Slug: "api", Name: "API"},
			{Slug: "web", Name: "Web"},
		},
		SpikeAmber:    3,
		SpikeRed:      10,
		DedupeWindow:  10 * time.Minute,
		BucketMinutes: 15,
	}
	st, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	srv, err := httpapi.New(cfg, st, m, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return srv.Handler()
}

func csrf(t *testing.T, h http.Handler) (*http.Cookie, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec, req)
	res := rec.Result()
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	const needle = `name="csrf" value="`
	i := strings.Index(string(body), needle)
	if i < 0 {
		t.Fatalf("csrf field missing: %s", body)
	}
	rest := string(body)[i+len(needle):]
	token, _, ok := strings.Cut(rest, `"`)
	if !ok || token == "" {
		t.Fatal("csrf token empty")
	}
	var cookie *http.Cookie
	for _, c := range res.Cookies() {
		if c.Name == "pc_csrf" {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("csrf cookie missing")
	}
	return cookie, token
}

func postForm(h http.Handler, path string, form url.Values, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func get(h http.Handler, path string, cookies ...*http.Cookie) (int, string) {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	res := rec.Result()
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(body)
}

func reportForm(token string) url.Values {
	form := url.Values{}
	form.Set("csrf", token)
	form.Set("service", "api")
	form.Set("category", "down")
	form.Set("description", "ZZSECRET<b>MARKER")
	form.Set("email", "hidden-reporter@example.com")
	return form
}

func TestPublicDoesNotLeakAndAdminDoes(t *testing.T) {
	h := newApp(t, mail.Disabled{})
	cookie, token := csrf(t, h)
	rec := postForm(h, "/report", reportForm(token), cookie)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Report recorded") {
		t.Fatalf("submit %d %s", rec.Code, rec.Body.String())
	}
	status, body := get(h, "/")
	if status != http.StatusOK {
		t.Fatal(status)
	}
	if strings.Contains(body, "ZZSECRET") || strings.Contains(body, "hidden-reporter@example.com") {
		t.Fatal("public page leaked private report fields")
	}
	if !strings.Contains(body, "1 in the last hour, 1 in the last 24 hours.") {
		t.Fatalf("count missing:\n%s", body)
	}
	status, body = get(h, "/admin")
	if status != http.StatusSeeOther {
		t.Fatalf("admin without login: %d", status)
	}
	if strings.Contains(body, "ZZSECRET") {
		t.Fatal("login redirect leaked the description")
	}

	login := url.Values{}
	login.Set("csrf", token)
	login.Set("password", "test-password-123")
	rec = postForm(h, "/admin/login", login, cookie)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("login %d %s", rec.Code, rec.Body.String())
	}
	var admin *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "pc_admin" {
			admin = c
		}
	}
	if admin == nil {
		t.Fatal("admin cookie missing")
	}
	status, body = get(h, "/admin", cookie, admin)
	if status != http.StatusOK {
		t.Fatal(status)
	}
	if !strings.Contains(body, "ZZSECRET") || !strings.Contains(body, "hidden-reporter@example.com") {
		t.Fatal("admin page missing private fields")
	}
	if strings.Contains(body, "<b>") {
		t.Fatal("description rendered as HTML")
	}
	if !strings.Contains(body, "skipped") {
		t.Fatal("expected skipped mail status")
	}
}

func TestDuplicateDoesNotInflateCount(t *testing.T) {
	h := newApp(t, mail.Disabled{})
	cookie, token := csrf(t, h)
	form := reportForm(token)
	if rec := postForm(h, "/report", form, cookie); rec.Code != http.StatusOK {
		t.Fatal(rec.Code, rec.Body.String())
	}
	rec := postForm(h, "/report", form, cookie)
	if rec.Code != http.StatusTooManyRequests || !strings.Contains(rec.Body.String(), "already reported") {
		t.Fatalf("duplicate %d %s", rec.Code, rec.Body.String())
	}
	_, body := get(h, "/")
	if !strings.Contains(body, "1 in the last hour, 1 in the last 24 hours.") {
		t.Fatalf("count changed:\n%s", body)
	}
}

func TestMailFailureStillCommits(t *testing.T) {
	m := &fakeMail{err: io.EOF}
	h := newApp(t, m)
	cookie, token := csrf(t, h)
	rec := postForm(h, "/report", reportForm(token), cookie)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Report recorded") {
		t.Fatalf("submit %d %s", rec.Code, rec.Body.String())
	}
	if m.sent != 1 {
		t.Fatalf("sent %d", m.sent)
	}
	_, body := get(h, "/")
	if !strings.Contains(body, "1 in the last hour, 1 in the last 24 hours.") || strings.Contains(body, "ZZSECRET") {
		t.Fatal("mail failure changed the public result")
	}
	login := url.Values{}
	login.Set("csrf", token)
	login.Set("password", "test-password-123")
	rec = postForm(h, "/admin/login", login, cookie)
	var admin *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "pc_admin" {
			admin = c
		}
	}
	_, body = get(h, "/admin", cookie, admin)
	if !strings.Contains(body, "failed") || !strings.Contains(body, "ZZSECRET") {
		t.Fatalf("admin missing failed report:\n%s", body)
	}
}

func TestHoneypotAndCSRF(t *testing.T) {
	h := newApp(t, mail.Disabled{})
	cookie, token := csrf(t, h)
	form := reportForm(token)
	form.Set("company", "spam llc")
	rec := postForm(h, "/report", form, cookie)
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Code)
	}
	_, body := get(h, "/")
	if !strings.Contains(body, "0 in the last hour, 0 in the last 24 hours.") {
		t.Fatal("honeypot report was counted")
	}

	bad := reportForm(token)
	bad.Set("csrf", "nope")
	rec = postForm(h, "/report", bad, cookie)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("csrf %d", rec.Code)
	}
	_, body = get(h, "/")
	if !strings.Contains(body, "0 in the last hour, 0 in the last 24 hours.") {
		t.Fatal("invalid csrf was counted")
	}
}
