// Package notify delivers event notifications to user-configured channels
// (Discord, ntfy, Slack, Telegram, Gotify, email, generic webhooks, ...) via
// Shoutrrr service URLs.
//
// Config comes from either the environment or the database. If
// VEERY_NOTIFY_URLS is set the whole config is env-managed and the UI treats it
// as read-only; otherwise it is edited in the UI and stored in settings.
package notify

import (
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	"github.com/SeriousBug/Veery/internal/api"
	"github.com/SeriousBug/Veery/internal/store"
	"github.com/containrrr/shoutrrr"
	"github.com/containrrr/shoutrrr/pkg/types"
)

// EnvURLs is the env var holding whitespace-separated Shoutrrr service URLs.
const EnvURLs = "VEERY_NOTIFY_URLS"

// EnvEvents is the env var holding a comma-separated list of the events to
// deliver. Unset means every event.
const EnvEvents = "VEERY_NOTIFY_EVENTS"

// Notifier sends notifications to the configured channels.
type Notifier struct {
	st *store.Store
	// env is non-nil when VEERY_NOTIFY_URLS is set, in which case it wins over
	// whatever is stored in the database.
	env *api.NotificationConfig
}

// New builds a Notifier, reading the env config once at startup.
func New(st *store.Store) *Notifier {
	n := &Notifier{st: st}
	if raw := os.Getenv(EnvURLs); strings.TrimSpace(raw) != "" {
		cfg := api.NotificationConfig{
			URLs:       strings.Fields(raw),
			Events:     envEvents(os.Getenv(EnvEvents)),
			EnvManaged: true,
		}
		n.env = &cfg
		log.Printf("notifications: %d target(s) configured from %s", len(cfg.URLs), EnvURLs)
	}
	return n
}

// envEvents turns a comma-separated event list into an enabled/disabled map.
// An empty list leaves every event enabled.
func envEvents(raw string) map[api.NotificationEvent]bool {
	out := map[api.NotificationEvent]bool{}
	if strings.TrimSpace(raw) == "" {
		return out
	}
	for _, ev := range api.AllNotificationEvents {
		out[ev] = false
	}
	for _, name := range strings.Split(raw, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, known := out[api.NotificationEvent(name)]; !known {
			log.Printf("notifications: ignoring unknown event %q in %s", name, EnvEvents)
			continue
		}
		out[api.NotificationEvent(name)] = true
	}
	return out
}

// Config returns the active config: the env one if present, else the stored one.
func (n *Notifier) Config() (api.NotificationConfig, error) {
	if n.env != nil {
		return *n.env, nil
	}
	return n.st.LoadNotificationConfig()
}

// ErrEnvManaged is returned when the config is pinned by the environment.
var ErrEnvManaged = errors.New("notifications are configured by " + EnvURLs + " and cannot be changed here")

// Save validates and persists the config. It fails if the config is env-managed.
func (n *Notifier) Save(cfg api.NotificationConfig) error {
	if n.env != nil {
		return ErrEnvManaged
	}
	cfg.URLs = cleanURLs(cfg.URLs)
	if err := Validate(cfg.URLs); err != nil {
		return err
	}
	if cfg.Events == nil {
		cfg.Events = map[api.NotificationEvent]bool{}
	}
	cfg.EnvManaged = false
	return n.st.SaveNotificationConfig(cfg)
}

// Validate reports the first URL Shoutrrr cannot build a sender for.
func Validate(urls []string) error {
	for _, u := range urls {
		if _, err := shoutrrr.CreateSender(u); err != nil {
			return fmt.Errorf("%s: %w", redact(u), err)
		}
	}
	return nil
}

// Notify delivers an event to every configured channel, unless that event is
// switched off. It returns immediately; delivery happens in the background and
// failures are logged, since no event is worth blocking a Docker action or an
// HTTP response on.
func (n *Notifier) Notify(ev api.NotificationEvent, title, body string) {
	cfg, err := n.Config()
	if err != nil {
		log.Printf("notifications: load config: %v", err)
		return
	}
	if len(cfg.URLs) == 0 || !cfg.Enabled(ev) {
		return
	}
	go func() {
		if err := Send(cfg.URLs, title, body); err != nil {
			log.Printf("notifications: send %s: %v", ev, err)
		}
	}()
}

// SendTest delivers a test message to urls, or to the saved targets when urls
// is empty. Unlike Notify it is synchronous and reports delivery failures, so
// the UI can tell the user whether their webhook actually works.
func (n *Notifier) SendTest(urls []string) error {
	urls = cleanURLs(urls)
	if len(urls) == 0 {
		cfg, err := n.Config()
		if err != nil {
			return err
		}
		urls = cfg.URLs
	}
	if len(urls) == 0 {
		return errors.New("no notification targets configured")
	}
	return Send(urls, "Veery test notification", "Notifications are working. This is a test from Veery.")
}

// Send delivers one message to every URL, joining the per-URL failures.
func Send(urls []string, title, body string) error {
	var failures []error
	for _, u := range urls {
		sender, err := shoutrrr.CreateSender(u)
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", redact(u), err))
			continue
		}
		msg := body
		if inlinesTitle(u) {
			msg = title + "\n\n" + body
		}
		params := types.Params{"title": title}
		for _, err := range sender.Send(msg, &params) {
			if err != nil {
				failures = append(failures, fmt.Errorf("%s: %w", redact(u), err))
			}
		}
	}
	return errors.Join(failures...)
}

// inlinesTitle reports whether a target drops the title and so needs it folded
// into the message body. Shoutrrr's generic webhook posts the bare message as
// the request body unless it is asked for a JSON payload with ?template=json —
// and the generic+http(s):// form cannot ask for one at all, because shoutrrr
// forwards that query to the target instead of reading it as config. Without
// this, a plain webhook would receive "the container is crash-looping" with no
// mention of which container.
func inlinesTitle(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	scheme, _, _ := strings.Cut(u.Scheme, "+")
	if scheme != "generic" {
		return false
	}
	if u.Scheme != "generic" {
		return true
	}
	return !strings.EqualFold(u.Query().Get("template"), "json")
}

func cleanURLs(urls []string) []string {
	out := make([]string, 0, len(urls))
	for _, u := range urls {
		if u = strings.TrimSpace(u); u != "" {
			out = append(out, u)
		}
	}
	return out
}

// redact strips everything but the scheme from a service URL: the rest is a
// webhook token or password and must not reach the logs.
func redact(url string) string {
	scheme, _, ok := strings.Cut(url, "://")
	if !ok {
		return "notification target"
	}
	return scheme + " target"
}
