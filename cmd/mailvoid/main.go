// Command mailvoid runs the single-binary MailVoid SMTP capture service and
// administrative dashboard.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gnrfan/mailvoid/internal/auth"
	"github.com/gnrfan/mailvoid/internal/build"
	"github.com/gnrfan/mailvoid/internal/config"
	"github.com/gnrfan/mailvoid/internal/events"
	"github.com/gnrfan/mailvoid/internal/policy"
	"github.com/gnrfan/mailvoid/internal/search"
	"github.com/gnrfan/mailvoid/internal/smtpserver"
	"github.com/gnrfan/mailvoid/internal/storage"
	"github.com/gnrfan/mailvoid/internal/web"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("mailvoid ")

	for _, arg := range os.Args[1:] {
		if arg == "--version" || arg == "-version" {
			fmt.Printf("mailvoid %s (commit %s, built %s)\n", build.Version, build.Commit, build.Date)
			return
		}
	}

	cfg, err := config.Load(os.Args[1:], os.Getenv)
	if err != nil {
		log.Fatalf("configuration: %v", err)
	}
	if err := run(cfg); err != nil {
		log.Fatalf("%v", err)
	}
}

func run(cfg config.Config) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return runContext(ctx, cfg)
}

func runContext(ctx context.Context, cfg config.Config) error {
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}

	baseStore, err := storage.NewGenji(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() { _ = baseStore.Close() }()

	fileSink, err := events.NewFileSink(cfg.EventLogPath)
	if err != nil {
		return fmt.Errorf("open event log: %w", err)
	}
	defer func() { _ = fileSink.Close() }()

	bus := events.NewBus(fileSink, baseStore)
	var searchIndex *search.Bleve
	store := storage.Store(baseStore)
	if cfg.SearchEnabled {
		searchIndex, _, err = search.Open(cfg.SearchIndexPath)
		if err != nil {
			return fmt.Errorf("open search index: %w", err)
		}
		defer func() { _ = searchIndex.Close() }()
		if err := searchIndex.Ensure(context.Background(), baseStore); err != nil {
			return fmt.Errorf("prepare search index: %w", err)
		}
		store = search.NewIndexedStore(baseStore, searchIndex, func(indexErr error) {
			bus.Emit(events.New(events.TypeSearchError, events.SeverityError, map[string]any{
				"error": indexErr.Error(),
			}))
		})
	}

	engine := policy.New(store)

	bootstrap, err := auth.New(store).EnsureBootstrap(context.Background(), cfg.AdminUser, cfg.AdminPassword, cfg.AdminResetPassword)
	if err != nil {
		return fmt.Errorf("bootstrap administrator: %w", err)
	}
	switch {
	case bootstrap.Created && bootstrap.GeneratedPass != "":
		log.Printf("created administrator %q with generated password: %s", bootstrap.Username, bootstrap.GeneratedPass)
		log.Printf("store this password now; change it in Settings after signing in")
	case bootstrap.Created:
		log.Printf("created administrator %q from configured credentials", bootstrap.Username)
	case bootstrap.Updated:
		log.Printf("reset password for administrator %q", bootstrap.Username)
	}

	smtpSrv := smtpserver.New(cfg, store, engine, bus)
	if err := smtpSrv.Start(); err != nil {
		return err
	}
	defer func() { _ = smtpSrv.Close() }()

	for _, addr := range smtpSrv.Addrs() {
		log.Printf("SMTP listening on %s", addr)
	}

	webSrv, err := web.New(cfg, store, bus, engine, build.Version)
	if err != nil {
		return fmt.Errorf("web server: %w", err)
	}
	webSrv.SetSearchIndex(searchIndex)

	sweepCtx, stopSweep := context.WithCancel(context.Background())
	defer stopSweep()
	go sweepExpired(sweepCtx, store, bus, cfg.SweepInterval)

	bus.Emit(events.New(events.TypeSystemStartup, events.SeverityInfo, map[string]any{
		"version":      build.Version,
		"smtp_ports":   cfg.SMTPPorts,
		"http_addr":    cfg.HTTPAddr,
		"domains":      cfg.Domains,
		"data_dir":     cfg.DataDir,
		"db_path":      cfg.DBPath,
		"event_log":    cfg.EventLogPath,
		"search_index": cfg.SearchIndexPath,
		"search":       cfg.SearchEnabled,
		"admin_user":   cfg.AdminUser,
		"started_at":   time.Now().UTC().Format(time.RFC3339),
		"max_message":  cfg.MaxMessageBytes,
	}))

	webErr := make(chan error, 1)
	go func() {
		log.Printf("HTTP dashboard listening on %s", cfg.HTTPAddr)
		webErr <- webSrv.Start()
	}()

	var runErr error
	select {
	case <-ctx.Done():
		log.Printf("shutdown signal received")
	case err := <-webErr:
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("HTTP server stopped: %v", err)
			runErr = err
		}
	}

	stopSweep()
	bus.Emit(events.New(events.TypeSystemShutdown, events.SeverityInfo, map[string]any{
		"version": build.Version,
		"reason":  "shutdown",
	}))

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := webSrv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("HTTP shutdown: %v", err)
	}
	if err := smtpSrv.Close(); err != nil {
		log.Printf("SMTP shutdown: %v", err)
	}
	log.Printf("goodbye")
	return runErr
}

func sweepExpired(ctx context.Context, store storage.Store, bus *events.Bus, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	sweep := func() {
		expired, err := store.DeleteExpired(ctx, time.Now().UTC())
		if err != nil {
			bus.Emit(events.New(events.TypeMessageError, events.SeverityError, map[string]any{
				"error":  err.Error(),
				"reason": "ttl sweep",
			}))
			return
		}
		for _, msg := range expired {
			bus.Emit(events.New(events.TypeMessageDeleted, events.SeverityInfo, map[string]any{
				"message_id": msg.ID,
				"recipient":  msg.Recipient,
				"mailbox":    msg.Mailbox,
				"domain":     msg.Domain,
				"local_port": msg.LocalPort,
				"remote_ip":  msg.RemoteIP,
				"subject":    msg.Subject,
				"action":     msg.Action,
				"policy":     msg.PolicyName,
				"reason":     "ttl expiry",
			}))
		}
	}

	sweep()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sweep()
		}
	}
}
