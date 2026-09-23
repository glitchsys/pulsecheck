package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"pulsecheck/internal/config"
	"pulsecheck/internal/httpapi"
	"pulsecheck/internal/mail"
	"pulsecheck/internal/store"
)

func main() {
	configPath := ""
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-config", "--config":
			if i+1 >= len(args) {
				fail(errors.New("-config requires a file path"))
			}
			i++
			configPath = args[i]
		case "-h", "-help", "--help":
			fmt.Fprintf(os.Stderr, "Usage: pulsecheck [-config file]\n\nEnvironment variables override the file. See .env.example.\n")
			return
		default:
			fail(fmt.Errorf("unknown argument %q", args[i]))
		}
	}
	if configPath == "" {
		configPath = os.Getenv("PULSECHECK_CONFIG")
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	cfg, err := config.Load(configPath)
	if err != nil {
		fail(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Info("connecting to database", "driver", cfg.Driver)
	st, err := store.Open(ctx, cfg)
	if err != nil {
		fail(err)
	}
	defer st.Close()

	srv, err := httpapi.New(cfg, st, mail.New(cfg.SMTP), log)
	if err != nil {
		fail(err)
	}
	httpSrv := &http.Server{
		Addr:              cfg.Addr(),
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      25 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.Addr(), "driver", cfg.Driver)
		errCh <- httpSrv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			fail(err)
		}
	case <-ctx.Done():
		log.Info("shutting down")
		shCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpSrv.Shutdown(shCtx); err != nil {
			fail(err)
		}
	}
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "pulsecheck: %v\n", err)
	os.Exit(1)
}
