// Command harborclass is the HarborClass Booking & Dispatch server.
// It is designed to be started exclusively via `docker compose up`.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/eaglepoint/harborclass/internal/audit"
	"github.com/eaglepoint/harborclass/internal/auth"
	"github.com/eaglepoint/harborclass/internal/bootstrap"
	"github.com/eaglepoint/harborclass/internal/config"
	"github.com/eaglepoint/harborclass/internal/dispatch"
	harborhttp "github.com/eaglepoint/harborclass/internal/http"
	"github.com/eaglepoint/harborclass/internal/notify"
	"github.com/eaglepoint/harborclass/internal/order"
	"github.com/eaglepoint/harborclass/internal/store"
)

func main() {
	gin.SetMode(gin.ReleaseMode)
	cfg := config.Load()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var s store.Store
	pg, err := store.OpenPostgres(ctx, cfg.DatabaseURL, cfg.MigrationsPath)
	if err != nil {
		// Production deployments (HARBORCLASS_REQUIRE_DB=true — the default
		// under `docker compose up`) treat a missing Postgres as a hard
		// failure so the documented runtime contract is honoured.
		// Developers running without compose can set the flag to false to
		// fall back to the in-memory store for quick local iteration.
		if cfg.RequireDatabase {
			log.Fatalf("postgres unavailable and HARBORCLASS_REQUIRE_DB=true: %v", err)
		}
		log.Printf("postgres unavailable, falling back to in-memory store: %v", err)
		s = store.NewMemory()
	} else {
		s = pg
	}

	if cfg.SeedDemoData {
		if err := bootstrap.Seed(ctx, s, cfg); err != nil {
			log.Printf("seed warning: %v", err)
		}
	}

	authSvc := auth.NewService(s)
	machine := order.NewMachine()
	chain := audit.New(s)
	engine := notify.NewEngine(s, &notify.LocalSender{})
	engine.ReminderCap = cfg.ReminderDailyCap
	engine.MaxAttempts = cfg.RetryMaxAttempts
	engine.BaseBackoff = cfg.RetryBaseDelay

	r := harborhttp.NewRouter(harborhttp.Dependencies{
		Config:   cfg,
		Store:    s,
		Auth:     authSvc,
		Machine:  machine,
		Engine:   engine,
		Chain:    chain,
		Strategy: dispatch.StrategyDistance,
	})

	srv := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("HarborClass listening on %s", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server: %v", err)
		}
	}()

	<-ctx.Done()
	shutdown, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	_ = srv.Shutdown(shutdown)
	log.Print("HarborClass stopped")
}
