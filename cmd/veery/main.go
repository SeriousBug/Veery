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

	"github.com/SeriousBug/Veery/internal/auth"
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	httpServer.Shutdown(shutdownCtx)
	log.Printf("shutdown complete")
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
