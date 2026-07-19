// Command veery is a self-hosted Docker manager with passkey-only auth.
package main

import (
	"context"
	"errors"
	"flag"
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
	"github.com/SeriousBug/Veery/internal/notify"
	"github.com/SeriousBug/Veery/internal/raidwatch"
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

	// Recovery: `veery invite [--admin]` mints a fresh enrollment link from the
	// host and exits. This is the account-recovery path if every passkey is
	// lost — host access already equals full control via the Docker socket.
	if len(os.Args) > 1 && os.Args[1] == "invite" {
		isAdmin := true
		for _, a := range os.Args[2:] {
			if a == "--normal" {
				isAdmin = false
			}
		}
		token, _, err := auth.NewInvite(st, "", "", isAdmin)
		if err != nil {
			log.Fatalf("mint invite: %v", err)
		}
		kind := "admin"
		if !isAdmin {
			kind = "normal"
		}
		log.Printf("%s enrollment link (valid 24h, single use):\n\n    %s\n", kind, auth.InviteURL(origin, token))
		return
	}

	// The helper container spawned by a self-update runs this. It performs the
	// swap from outside the Veery container being replaced, because that swap
	// stops the container it is replacing, and a process cannot outlive the
	// container it is running in.
	if len(os.Args) > 1 && os.Args[1] == "apply-update" {
		if err := applyUpdate(st, os.Args[2:]); err != nil {
			log.Fatalf("apply-update: %v", err)
		}
		return
	}

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

	notifier := notify.New(st)
	srv.SetNotifier(notifier)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dkr, err := docker.NewManager(st, srv.Hub())
	if err != nil {
		log.Printf("warning: docker manager: %v", err)
	} else {
		defer dkr.Close()
		srv.SetDocker(dkr)
		dkr.SetNotifier(notifier)
		if err := dkr.Ping(ctx); err != nil {
			log.Printf("warning: docker daemon unreachable: %v", err)
		}
		// Settle anything a previous run left half-done (a crash mid-update, or
		// the self-update that just replaced that run) before serving, then pick
		// up whatever the user changed on the host while it was down.
		dkr.Recover(ctx)
		dkr.Reconcile(ctx)
		go pollStacks(ctx, dkr, st)
		go pollMetrics(ctx, dkr, srv.Hub(), st)
		go dkr.AutoUpdatePoller(ctx)
		go dkr.UpdateCheckPoller(ctx)
	}

	// The RAID watcher reads /proc and /sys, not Docker, so it runs regardless
	// of whether the Docker manager came up. It no-ops on hosts without md
	// arrays or the mounts.
	go raidwatch.New(st, notifier).Poller(ctx)

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

// applyUpdate runs the swap for a container from the outside, as the detached
// helper container started by a self-update. It has no HTTP server and no WS:
// progress goes to the DB, which the replacement Veery reads back and reports.
func applyUpdate(st *store.Store, args []string) error {
	fs := flag.NewFlagSet("apply-update", flag.ExitOnError)
	name := fs.String("container", "", "container to update")
	jobID := fs.String("job", "", "update job id to report progress under")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" {
		return errors.New("--container is required")
	}

	dkr, err := docker.NewManager(st, nil)
	if err != nil {
		return err
	}
	defer dkr.Close()
	dkr.SetNotifier(notify.New(st))

	// Deliberately not tied to SIGTERM: the swap must run to completion or roll
	// back, and being interrupted halfway is what leaves a service down.
	ctx := context.Background()
	log.Printf("applying update to %s", *name)
	return dkr.ApplyUpdate(ctx, *name, *jobID)
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
// transitions reach connected clients, reconciling first so what it pushes
// accounts for containers the user has added, removed or recreated by hand.
func pollStacks(ctx context.Context, dkr *docker.Manager, st *store.Store) {
	dkr.BroadcastStacks(ctx)
	for {
		t := time.NewTimer(pollInterval(st))
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
			dkr.Reconcile(ctx)
			dkr.BroadcastStacks(ctx)
		}
	}
}

// pollMetrics builds a host+container metrics snapshot on an interval and
// broadcasts it over the WS.
func pollMetrics(ctx context.Context, dkr *docker.Manager, hub *server.Hub, st *store.Store) {
	col := metrics.New()
	peaks, err := st.LoadDiskPeaks()
	if err != nil {
		log.Printf("metrics: load disk peaks: %v", err)
		peaks = map[string]metrics.DevicePeak{}
	}
	for {
		t := time.NewTimer(pollInterval(st))
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
		sample := col.Sample()
		if metrics.UpdatePeaks(peaks, sample.Devices) {
			if err := st.SaveDiskPeaks(peaks); err != nil {
				log.Printf("metrics: save disk peaks: %v", err)
			}
		}
		vis, err := st.LoadDiskVisibility()
		if err != nil {
			log.Printf("metrics: load disk visibility: %v", err)
		}
		host := metrics.BuildHostMetrics(sample, vis, peaks)
		host.Mdadm = metrics.ScanMdadm()
		if len(host.Mdadm) > 0 {
			if lastScan, err := st.LoadMdadmLastScan(); err == nil {
				for i := range host.Mdadm {
					host.Mdadm[i].LastScanAt = lastScan[host.Mdadm[i].Name]
				}
			} else {
				log.Printf("metrics: load mdadm last scan: %v", err)
			}
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
