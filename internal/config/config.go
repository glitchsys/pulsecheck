package config

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// Service is one product area visitors can report against.
type Service struct {
	Slug string
	Name string
}

// SMTP is optional support mail. Host empty means mail is off.
type SMTP struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	To       []string
}

func (s SMTP) Enabled() bool { return s.Host != "" }

// Config is the process configuration. Environment variables override a file.
type Config struct {
	Port              string
	Driver            string
	SQLitePath        string
	DatabaseURL       string
	AdminPassword     string
	SessionSecret     string
	IPHashPepper      string
	ProductName       string
	OfficialStatusURL string
	Services          []Service
	SpikeAmber        int
	SpikeRed          int
	DedupeWindow      time.Duration
	BucketMinutes     int
	SMTP              SMTP
	TrustProxy        bool
	CookieSecure      bool
}

// Addr is the listen address, including a leading colon.
func (c Config) Addr() string {
	p := c.Port
	if p == "" {
		p = "8080"
	}
	if strings.HasPrefix(p, ":") {
		return p
	}
	return ":" + p
}

// Load reads an optional KEY=VALUE file, then applies the environment.
// A variable already set in the environment wins over the file.
func Load(path string) (Config, error) {
	file := map[string]string{}
	if path != "" {
		text, err := os.ReadFile(path)
		if err != nil {
			return Config{}, fmt.Errorf("read config %s: %w", path, err)
		}
		parsed, err := parseFile(string(text))
		if err != nil {
			return Config{}, fmt.Errorf("parse config %s: %w", path, err)
		}
		file = parsed
	}
	lookup := func(key string) string {
		if v, ok := os.LookupEnv(key); ok {
			return v
		}
		return file[key]
	}

	driver := strings.ToLower(strings.TrimSpace(lookup("DB_DRIVER")))
	if driver == "" {
		driver = "sqlite"
	}
	if driver != "sqlite" && driver != "mysql" {
		return Config{}, fmt.Errorf("DB_DRIVER must be sqlite or mysql, got %q", driver)
	}

	sqlitePath := strings.TrimSpace(lookup("SQLITE_PATH"))
	if sqlitePath == "" {
		sqlitePath = "./data/pulsecheck.db"
	}
	databaseURL := strings.TrimSpace(lookup("DATABASE_URL"))
	if driver == "mysql" && databaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required when DB_DRIVER=mysql")
	}

	admin := lookup("ADMIN_PASSWORD")
	if len(admin) < 8 {
		return Config{}, fmt.Errorf("ADMIN_PASSWORD must be at least 8 characters")
	}
	session := lookup("SESSION_SECRET")
	if len(session) < 16 {
		return Config{}, fmt.Errorf("SESSION_SECRET must be at least 16 characters")
	}
	pepper := lookup("IP_HASH_PEPPER")
	if len(pepper) < 16 {
		return Config{}, fmt.Errorf("IP_HASH_PEPPER must be at least 16 characters")
	}

	product := strings.TrimSpace(lookup("PRODUCT_NAME"))
	if product == "" {
		product = "PulseCheck"
	}
	if len(product) > 80 || strings.ContainsAny(product, "\r\n") {
		return Config{}, fmt.Errorf("PRODUCT_NAME must be 1–80 characters and a single line")
	}

	official := strings.TrimSpace(lookup("OFFICIAL_STATUS_URL"))
	if official != "" {
		u, err := url.Parse(official)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return Config{}, fmt.Errorf("OFFICIAL_STATUS_URL must be an http or https URL")
		}
	}

	services, err := parseServices(lookup("SERVICES"))
	if err != nil {
		return Config{}, err
	}

	amber, err := parseIntDefault(lookup("SPIKE_THRESHOLD_AMBER"), 3)
	if err != nil || amber < 1 {
		return Config{}, fmt.Errorf("SPIKE_THRESHOLD_AMBER must be a positive integer")
	}
	red, err := parseIntDefault(lookup("SPIKE_THRESHOLD_RED"), 10)
	if err != nil || red <= amber {
		return Config{}, fmt.Errorf("SPIKE_THRESHOLD_RED must be an integer greater than SPIKE_THRESHOLD_AMBER")
	}

	dedupeMin, err := parseIntDefault(lookup("DEDUPE_WINDOW_MINUTES"), 10)
	if err != nil || dedupeMin < 1 || dedupeMin > 1440 {
		return Config{}, fmt.Errorf("DEDUPE_WINDOW_MINUTES must be between 1 and 1440")
	}
	bucket, err := parseIntDefault(lookup("BUCKET_MINUTES"), 15)
	if err != nil || !validBucket(bucket) {
		return Config{}, fmt.Errorf("BUCKET_MINUTES must be one of 5, 10, 15, 20, 30, 60")
	}

	port := strings.TrimSpace(lookup("PORT"))
	if port == "" {
		port = "8080"
	}
	portNum := strings.TrimPrefix(port, ":")
	n, err := strconv.Atoi(portNum)
	if err != nil || n < 1 || n > 65535 {
		return Config{}, fmt.Errorf("PORT must be a TCP port")
	}

	smtp, err := parseSMTP(lookup)
	if err != nil {
		return Config{}, err
	}
	trust, err := parseBoolDefault(lookup("TRUST_PROXY"), false)
	if err != nil {
		return Config{}, fmt.Errorf("TRUST_PROXY: %w", err)
	}
	secure, err := parseBoolDefault(lookup("COOKIE_SECURE"), false)
	if err != nil {
		return Config{}, fmt.Errorf("COOKIE_SECURE: %w", err)
	}

	return Config{
		Port:              port,
		Driver:            driver,
		SQLitePath:        sqlitePath,
		DatabaseURL:       databaseURL,
		AdminPassword:     admin,
		SessionSecret:     session,
		IPHashPepper:      pepper,
		ProductName:       product,
		OfficialStatusURL: official,
		Services:          services,
		SpikeAmber:        amber,
		SpikeRed:          red,
		DedupeWindow:      time.Duration(dedupeMin) * time.Minute,
		BucketMinutes:     bucket,
		SMTP:              smtp,
		TrustProxy:        trust,
		CookieSecure:      secure,
	}, nil
}

func parseSMTP(lookup func(string) string) (SMTP, error) {
	host := strings.TrimSpace(lookup("SMTP_HOST"))
	if host == "" {
		return SMTP{}, nil
	}
	port, err := parseIntDefault(lookup("SMTP_PORT"), 587)
	if err != nil || port < 1 || port > 65535 {
		return SMTP{}, fmt.Errorf("SMTP_PORT must be a TCP port")
	}
	from := strings.TrimSpace(lookup("SMTP_FROM"))
	if !validEmail(from) {
		return SMTP{}, fmt.Errorf("SMTP_FROM must be an email address when SMTP_HOST is set")
	}
	rawTo := strings.TrimSpace(lookup("SUPPORT_EMAIL"))
	if rawTo == "" {
		return SMTP{}, fmt.Errorf("SUPPORT_EMAIL is required when SMTP_HOST is set")
	}
	var to []string
	for _, part := range strings.Split(rawTo, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if !validEmail(part) {
			return SMTP{}, fmt.Errorf("SUPPORT_EMAIL contains an invalid address")
		}
		to = append(to, part)
	}
	if len(to) == 0 {
		return SMTP{}, fmt.Errorf("SUPPORT_EMAIL is required when SMTP_HOST is set")
	}
	return SMTP{
		Host:     host,
		Port:     port,
		Username: lookup("SMTP_USER"),
		Password: lookup("SMTP_PASSWORD"),
		From:     from,
		To:       to,
	}, nil
}

func parseServices(raw string) ([]Service, error) {
	seen := map[string]bool{}
	var out []Service
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		slug, name, ok := strings.Cut(part, ":")
		if !ok {
			return nil, fmt.Errorf("SERVICES entry %q must look like slug:Name", part)
		}
		slug = strings.ToLower(strings.TrimSpace(slug))
		name = strings.TrimSpace(name)
		if !slugPattern.MatchString(slug) || strings.HasSuffix(slug, "-") {
			return nil, fmt.Errorf("service slug %q must be lowercase letters, numbers, and hyphens", slug)
		}
		if name == "" || len(name) > 80 || strings.ContainsAny(name, "\r\n") {
			return nil, fmt.Errorf("service name for %q must be a single line of 1–80 characters", slug)
		}
		if seen[slug] {
			return nil, fmt.Errorf("duplicate service slug %q", slug)
		}
		seen[slug] = true
		out = append(out, Service{Slug: slug, Name: name})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("SERVICES is required, for example web:Web application,api:API")
	}
	return out, nil
}

func parseFile(text string) (map[string]string, error) {
	out := map[string]string{}
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		key, val, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" || strings.ContainsAny(key, " \t") {
			return nil, fmt.Errorf("line %d: expected KEY=VALUE", i+1)
		}
		val = strings.TrimSpace(val)
		if len(val) >= 2 {
			q := val[0]
			if (q == '"' || q == '\'') && val[len(val)-1] == q {
				val = val[1 : len(val)-1]
			}
		}
		out[key] = val
	}
	return out, nil
}

func parseIntDefault(raw string, fallback int) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback, nil
	}
	return strconv.Atoi(raw)
}

func parseBoolDefault(raw string, fallback bool) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return fallback, nil
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("must be true or false")
	}
}

func validBucket(n int) bool {
	switch n {
	case 5, 10, 15, 20, 30, 60:
		return true
	default:
		return false
	}
}

func validEmail(s string) bool {
	if len(s) < 3 || len(s) > 254 || strings.ContainsAny(s, " \r\n\t") {
		return false
	}
	at := strings.IndexByte(s, '@')
	if at <= 0 || at >= len(s)-1 {
		return false
	}
	return !strings.Contains(s[at+1:], "@")
}
