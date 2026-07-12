// Command veery is a self-hosted Docker manager with passkey-only auth.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/SeriousBug/Veery/internal/api"
	"github.com/SeriousBug/Veery/internal/auth"
	"github.com/SeriousBug/Veery/internal/docker"
	"github.com/SeriousBug/Veery/internal/metrics"
	"github.com/SeriousBug/Veery/internal/server"
	"github.com/SeriousBug/Veery/internal/store"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("veery ")

	dbPath := env("VEERY_DB", "/data/veery.db")
	addr := env("VEERY_ADDR", ":8080")
	origin := env("VEERY_ORIGIN", "http://localhost:8080")
	rpID := env("VEERY_RP_ID", "localhost")

	st, err := store.Open(dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	authMgr, err := auth.NewManager(st, auth.Config{RPID: rpID, Origin: origin})
	if err != nil {
		log.Fatalf("auth manager: %v", err)
	}

	if url, err := auth.Bootstrap(st, origin); err != nil {
		log.Fatalf("bootstrap: %v", err)
	} else if url != "" {
		log.Printf("no users yet — enroll the first admin passkey here:\n\n    %s\n", url)
	}

	srv := server.New(st, authMgr, server.Config{
		RPID:   rpID,
		Origin: origin,
		Secure: strings.HasPrefix(origin, "https://"),
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dkr, err := docker.NewManager(st, srv.Hub())
	if err != nil {
		log.Printf("warning: docker manager: %v", err)
	} else {
		defer dkr.Close()
		srv.SetDocker(dkr)
		if err := dkr.Ping(ctx); err != nil {
			log.Printf("warning: docker daemon unreachable: %v", err)
		}
		go pollStacks(ctx, dkr, st)
		go pollMetrics(ctx, dkr, srv.Hub(), st)
		go dkr.AutoUpdatePoller(ctx)
	}

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("listening on %s", addr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server: %v", err)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	httpServer.Shutdown(shutdownCtx)
	log.Printf("shutdown complete")
}

// pollInterval reads the configured poll interval, defaulting to 5s.
func pollInterval(st *store.Store) time.Duration {
	secs := store.DefaultPollIntervalSeconds
	if cfg, err := st.LoadSettings(); err == nil && cfg.PollIntervalSeconds > 0 {
		secs = cfg.PollIntervalSeconds
	}
	return time.Duration(secs) * time.Second
}

// pollStacks pushes a fresh stacks list over the WS on an interval so status
// transitions reach connected clients.
func pollStacks(ctx context.Context, dkr *docker.Manager, st *store.Store) {
	dkr.BroadcastStacks(ctx)
	for {
		t := time.NewTimer(pollInterval(st))
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
			dkr.BroadcastStacks(ctx)
		}
	}
}

// pollMetrics builds a host+container metrics snapshot on an interval and
// broadcasts it over the WS.
func pollMetrics(ctx context.Context, dkr *docker.Manager, hub *server.Hub, st *store.Store) {
	col := metrics.New()
	for {
		t := time.NewTimer(pollInterval(st))
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
		host, err := col.Snapshot()
		if err != nil {
			log.Printf("metrics: host snapshot: %v", err)
		}
		containers, err := dkr.ContainerStats(ctx)
		if err != nil {
			log.Printf("metrics: container stats: %v", err)
		}
		snap := api.MetricsSnapshot{Host: host, Containers: containers, At: time.Now().Unix()}
		hub.Broadcast(api.WSMessage{Type: api.WSTypeMetrics, Metrics: &snap})
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
