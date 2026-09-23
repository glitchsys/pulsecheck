package mail

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"

	"pulsecheck/internal/config"
)

// Notice is the private support message for one stored report.
type Notice struct {
	ID            int64
	Product       string
	Component     string
	Category      string
	Description   string
	ReporterEmail string
	When          time.Time
	AdminURL      string
}

// Mailer sends support mail. Enabled is false when SMTP is not configured.
type Mailer interface {
	Enabled() bool
	Send(ctx context.Context, n Notice) error
}

// Disabled drops mail. Reports are still stored by the caller.
type Disabled struct{}

func (Disabled) Enabled() bool { return false }

func (Disabled) Send(context.Context, Notice) error {
	return errors.New("email disabled")
}

// New returns an SMTP mailer, or Disabled when SMTP_HOST is empty.
func New(smtpCfg config.SMTP) Mailer {
	if !smtpCfg.Enabled() {
		return Disabled{}
	}
	return &Client{cfg: smtpCfg}
}

// Client sends one message over SMTP during the request.
// See .claude/docs/landmines.md.
type Client struct {
	cfg config.SMTP
}

func (c *Client) Enabled() bool { return true }

func (c *Client) Send(ctx context.Context, n Notice) error {
	addr := net.JoinHostPort(c.cfg.Host, fmt.Sprintf("%d", c.cfg.Port))
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	raw, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer raw.Close()
	_ = raw.SetDeadline(time.Now().Add(8 * time.Second))

	if c.cfg.Port == 465 {
		tlsConn := tls.Client(raw, &tls.Config{ServerName: c.cfg.Host, MinVersion: tls.VersionTLS12})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return err
		}
		raw = tlsConn
	}

	client, err := smtp.NewClient(raw, c.cfg.Host)
	if err != nil {
		return err
	}
	defer client.Close()

	if c.cfg.Port != 465 {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: c.cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
				return err
			}
		} else if c.cfg.Host != "localhost" && c.cfg.Host != "127.0.0.1" {
			return errors.New("smtp server does not offer STARTTLS")
		}
	}
	if c.cfg.Username != "" {
		if err := client.Auth(smtp.PlainAuth("", c.cfg.Username, c.cfg.Password, c.cfg.Host)); err != nil {
			return err
		}
	}
	if err := client.Mail(c.cfg.From); err != nil {
		return err
	}
	for _, to := range c.cfg.To {
		if err := client.Rcpt(to); err != nil {
			return err
		}
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(Build(c.cfg.From, c.cfg.To, n)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

// Build formats a plain-text support message. Header fields cannot contain newlines.
func Build(from string, to []string, n Notice) []byte {
	when := n.When.UTC()
	if when.IsZero() {
		when = time.Now().UTC()
	}
	desc := n.Description
	if strings.TrimSpace(desc) == "" {
		desc = "(none)"
	}
	email := n.ReporterEmail
	if strings.TrimSpace(email) == "" {
		email = "(none)"
	}
	product := oneLine(n.Product)
	if product == "" {
		product = "PulseCheck"
	}
	subject := fmt.Sprintf("[%s] %s — %s", product, oneLine(n.Category), oneLine(n.Component))
	body := fmt.Sprintf(
		"This is one unverified customer report.\r\n\r\nReport ID: %d\r\nTime (UTC): %s\r\nComponent: %s\r\nCategory: %s\r\nReporter email: %s\r\nAdmin: %s\r\n\r\nDescription:\r\n%s\r\n",
		n.ID,
		when.Format(time.RFC3339),
		oneLine(n.Component),
		oneLine(n.Category),
		oneLine(email),
		oneLine(n.AdminURL),
		desc,
	)
	msg := fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		oneLine(from),
		oneLine(strings.Join(to, ", ")),
		subject,
		when.Format(time.RFC1123Z),
		body,
	)
	return []byte(msg)
}

func oneLine(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' {
			return -1
		}
		return r
	}, s)
}
