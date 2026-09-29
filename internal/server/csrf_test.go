package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/SeriousBug/Veery/internal/auth"
	"github.com/coder/websocket"
)

func loggedInAdmin(t *testing.T) (*httptest.Server, *http.Client) {
	t.Helper()
	ts, st, client := testServer(t)
	inviteURL, err := auth.Bootstrap(st, ts.URL)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	enroll(t, ts, client, inviteURL[strings.Index(inviteURL, "token=")+len("token="):], "admin")
	return ts, client
}

// A page on a sibling subdomain is same-site, so SameSite=Lax cookies ride
// along on its requests. Only same-origin requests may change state.
func TestCrossOriginUnsafeRequestsRejected(t *testing.T) {
	ts, client := loggedInAdmin(t)

	cases := []struct {
		name    string
		headers map[string]string
		want    int
	}{
		{"same-site subdomain", map[string]string{"Sec-Fetch-Site": "same-site", "Origin": "http://evil.example.com"}, http.StatusForbidden},
		{"cross-site", map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "http://evil.test"}, http.StatusForbidden},
		{"other origin without Sec-Fetch-Site", map[string]string{"Origin": "http://evil.example.com"}, http.StatusForbidden},
		{"same-origin", map[string]string{"Sec-Fetch-Site": "same-origin", "Origin": ts.URL}, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/invites", strings.NewReader(`{}`))
			req.Header.Set("Content-Type", "application/json")
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != tc.want {
				t.Fatalf("status %d, want %d", resp.StatusCode, tc.want)
			}
		})
	}
}

func TestWebSocketRejectsOtherOrigins(t *testing.T) {
	ts, client := loggedInAdmin(t)
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"

	dial := func(origin string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
			HTTPClient: client,
			HTTPHeader: http.Header{"Origin": {origin}},
		})
		if err == nil {
			conn.CloseNow()
		}
		return err
	}

	if err := dial("http://evil.example.com"); err == nil {
		t.Fatal("WS from another origin was accepted")
	}
	if err := dial(ts.URL); err != nil {
		t.Fatalf("WS from own origin: %v", err)
	}
}

// Under HTTPS, cookies use the __Host- prefix so a sibling subdomain cannot
// overwrite them with Domain=.example.com.
func TestSecureCookiesUseHostPrefix(t *testing.T) {
	s := &Server{cfg: Config{Secure: true}}
	rec := httptest.NewRecorder()
	s.setSessionCookie(rec, "tok", time.Now().Add(time.Hour))
	s.setCeremonyCookie(rec, "cid")

	for _, c := range rec.Result().Cookies() {
		if !strings.HasPrefix(c.Name, "__Host-") || !c.Secure || c.Path != "/" || c.Domain != "" {
			t.Errorf("cookie %q: secure=%v path=%q domain=%q", c.Name, c.Secure, c.Path, c.Domain)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: "tossed"})
	if _, ok := s.cookie(req, auth.SessionCookieName); ok {
		t.Error("unprefixed session cookie was accepted")
	}
}
