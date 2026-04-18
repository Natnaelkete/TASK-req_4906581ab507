package http_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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

// newHarness builds a fully wired router with a real in-memory store.
// No services are mocked: the same handlers, auth service, dispatch
// pipeline, notify engine, and audit chain run here and in production.
type harness struct {
	router *gin.Engine
	store  store.Store
	engine *notify.Engine
	sender *notify.LocalSender
	cfg    config.Config
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	gin.SetMode(gin.TestMode)
	notify.Sleep = func(_ time.Duration) {} // no-op so retry tests are instant
	cfg := config.Config{
		HTTPAddr:         ":0",
		EncryptionKey:    "harborclass-test-encryption-key-0",
		ReminderDailyCap: notify.DefaultReminderCap,
		RetryMaxAttempts: notify.DefaultMaxAttempts,
		RetryBaseDelay:   0,
		PickupCutoffHour: 20,
	}
	return build(t, cfg)
}

// actual builder — separated so we can build with a specific config.
func build(t *testing.T, cfg config.Config) *harness {
	t.Helper()
	s := store.NewMemory()
	if err := bootstrap.Seed(context.Background(), s, cfg); err != nil {
		t.Fatalf("seed: %v", err)
	}
	authSvc := auth.NewService(s)
	sender := &notify.LocalSender{}
	eng := notify.NewEngine(s, sender)
	eng.ReminderCap = cfg.ReminderDailyCap
	eng.MaxAttempts = cfg.RetryMaxAttempts
	eng.BaseBackoff = 0
	eng.JitterBackoff = 0
	chain := audit.New(s)
	r := harborhttp.NewRouter(harborhttp.Dependencies{
		Config:   cfg,
		Store:    s,
		Auth:     authSvc,
		Machine:  order.NewMachine(),
		Engine:   eng,
		Chain:    chain,
		Strategy: dispatch.StrategyDistance,
	})
	return &harness{router: r, store: s, engine: eng, sender: sender, cfg: cfg}
}

// loginAs authenticates a demo user and returns a bearer token.
func (h *harness) loginAs(t *testing.T, username, password string) string {
	t.Helper()
	body := strings.NewReader(`{"Username":"` + username + `","Password":"` + password + `"}`)
	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/auth/login", body)
	req.Header.Set("Content-Type", "application/json")
	h.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login %s failed: %d %s", username, rec.Code, rec.Body.String())
	}
	var resp struct{ Token string }
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse token: %v", err)
	}
	if resp.Token == "" {
		t.Fatalf("empty token, body=%s", rec.Body.String())
	}
	return resp.Token
}

// do is a tiny helper for issuing authenticated requests.
func (h *harness) do(t *testing.T, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(method, path, r)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	h.router.ServeHTTP(rec, req)
	return rec
}

func mustJSON(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("bad json: %v body=%s", err, string(body))
	}
	return m
}

// signUnsubscribe mints a valid signed unsubscribe token using the
// harness's encryption key — tests can now exercise the one-click
// flow without reproducing the HMAC logic inline.
func (h *harness) signUnsubscribe(user, category string) string {
	key := auth.DeriveKey(h.cfg.EncryptionKey)
	return auth.SignUnsubscribe(key, user, category, time.Now())
}
