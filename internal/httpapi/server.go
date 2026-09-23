package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"

	"pulsecheck/internal/config"
	"pulsecheck/internal/mail"
	"pulsecheck/internal/store"
	"pulsecheck/internal/web"
)

const (
	csrfCookie  = "pc_csrf"
	adminCookie = "pc_admin"
)

// Server serves the public chart, the report form, and the password admin.
type Server struct {
	cfg   config.Config
	store *store.Store
	mail  mail.Mailer
	tmpl  *template.Template
	log   *slog.Logger
}

// New parses embedded templates and returns the HTTP application.
func New(cfg config.Config, st *store.Store, m mail.Mailer, log *slog.Logger) (*Server, error) {
	if m == nil {
		m = mail.Disabled{}
	}
	if log == nil {
		log = slog.Default()
	}
	tmpl, err := template.ParseFS(web.Files, "templates/*.html")
	if err != nil {
		return nil, err
	}
	return &Server{cfg: cfg, store: st, mail: m, tmpl: tmpl, log: log}, nil
}

// Handler is the full application, including security headers.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.Handle("GET /static/", s.static())
	mux.HandleFunc("GET /{$}", s.home)
	mux.HandleFunc("POST /report", s.report)
	mux.HandleFunc("GET /admin/login", s.loginForm)
	mux.HandleFunc("POST /admin/login", s.login)
	mux.HandleFunc("POST /admin/logout", s.logout)
	mux.HandleFunc("GET /admin", s.admin)
	mux.HandleFunc("POST /admin/reports/{id}/delete", s.deleteReport)
	return s.wrap(mux)
}

func (s *Server) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; img-src 'self'; form-action 'self'; frame-ancestors 'none'; base-uri 'self'")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) static() http.Handler {
	sub, err := fsSub()
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "static files unavailable", http.StatusInternalServerError)
		})
	}
	files := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=3600")
		http.StripPrefix("/static/", files).ServeHTTP(w, r)
	})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Ping(r.Context()); err != nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func (s *Server) render(w http.ResponseWriter, name string, status int, data view) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := s.tmpl.ExecuteTemplate(w, name, data); err != nil {
		s.log.Error("template", "name", name, "err", err)
	}
}

func (s *Server) newView(w http.ResponseWriter, r *http.Request, title string) view {
	return view{
		Title:       title,
		Product:     s.cfg.ProductName,
		OfficialURL: s.cfg.OfficialStatusURL,
		CSRF:        s.csrfToken(w, r),
	}
}

func (s *Server) csrfToken(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie(csrfCookie); err == nil && len(c.Value) == 64 {
		return c.Value
	}
	buf := make([]byte, 32)
	if _, err := randRead(buf); err != nil {
		return ""
	}
	tok := hex.EncodeToString(buf)
	http.SetCookie(w, s.cookie(csrfCookie, tok, 86400))
	return tok
}

func (s *Server) csrfOK(r *http.Request) bool {
	c, err := r.Cookie(csrfCookie)
	if err != nil || len(c.Value) != 64 {
		return false
	}
	form := r.FormValue("csrf")
	if len(form) != len(c.Value) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(form), []byte(c.Value)) == 1
}

func (s *Server) adminOK(r *http.Request) bool {
	c, err := r.Cookie(adminCookie)
	if err != nil {
		return false
	}
	payload, ok := verify(s.cfg.SessionSecret, c.Value)
	if !ok {
		return false
	}
	exp, err := strconv.ParseInt(payload, 10, 64)
	if err != nil {
		return false
	}
	return timeNow().Unix() < exp
}

func (s *Server) cookie(name, value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cfg.CookieSecure,
		MaxAge:   maxAge,
	}
}

func (s *Server) baseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || s.cfg.CookieSecure {
		scheme = "https"
	}
	if s.cfg.TrustProxy {
		switch r.Header.Get("X-Forwarded-Proto") {
		case "https", "http":
			scheme = r.Header.Get("X-Forwarded-Proto")
		}
	}
	return scheme + "://" + r.Host
}

func clientIP(r *http.Request, trust bool) string {
	if trust {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			first := strings.TrimSpace(strings.Split(xff, ",")[0])
			if first != "" {
				return first
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func hashIP(pepper, ip string) string {
	mac := hmac.New(sha256.New, []byte(pepper))
	_, _ = mac.Write([]byte(ip))
	return hex.EncodeToString(mac.Sum(nil))
}

func passwordOK(given, want string) bool {
	g := sha256.Sum256([]byte(given))
	w := sha256.Sum256([]byte(want))
	return subtle.ConstantTimeCompare(g[:], w[:]) == 1
}

func sign(secret, payload string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	return payload + "." + hex.EncodeToString(mac.Sum(nil))
}

func verify(secret, token string) (string, bool) {
	payload, sigHex, ok := strings.Cut(token, ".")
	if !ok {
		return "", false
	}
	sig, err := hex.DecodeString(sigHex)
	if err != nil {
		return "", false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	if !hmac.Equal(mac.Sum(nil), sig) {
		return "", false
	}
	return payload, true
}
