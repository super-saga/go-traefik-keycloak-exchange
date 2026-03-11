package traefik_keycloak_exchange

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type errReadCloser struct{}

func (errReadCloser) Read([]byte) (int, error) {
	return 0, errors.New("read error")
}

func (errReadCloser) Close() error {
	return nil
}

func newTestMiddleware(t *testing.T, config *Config, name string) *Middleware {
	t.Helper()
	handler, err := New(context.Background(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), config, name)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	m, ok := handler.(*Middleware)
	if !ok {
		t.Fatalf("expected *Middleware, got %T", handler)
	}
	return m
}

func TestCreateConfigDefaults(t *testing.T) {
	config := CreateConfig()
	if config.ClientIDHeader != "X-Client-ID" {
		t.Fatalf("ClientIDHeader default mismatch: %s", config.ClientIDHeader)
	}
	if config.ClientSecretHeader != "X-Client-Secret" {
		t.Fatalf("ClientSecretHeader default mismatch: %s", config.ClientSecretHeader)
	}
	if !config.RequireClientCredentials {
		t.Fatal("RequireClientCredentials default mismatch")
	}
	if config.TokenRequestTimeoutSeconds != 10 {
		t.Fatalf("TokenRequestTimeoutSeconds default mismatch: %d", config.TokenRequestTimeoutSeconds)
	}
}

func TestNewValidationAndTimeout(t *testing.T) {
	if _, err := New(context.Background(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), nil, "test"); err == nil {
		t.Fatal("expected error for nil config")
	}
	if _, err := New(context.Background(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), &Config{}, "test"); err == nil {
		t.Fatal("expected error for empty keycloakURL")
	}

	config := CreateConfig()
	config.KeycloakURL = "https://keycloak.example.com/realms/test"
	config.TokenRequestTimeoutSeconds = 0
	m := newTestMiddleware(t, config, "test")
	if m.httpClient.Timeout != 10*time.Second {
		t.Fatalf("expected timeout 10s, got %v", m.httpClient.Timeout)
	}
}

func TestReadClientCredentials(t *testing.T) {
	config := CreateConfig()
	config.KeycloakURL = "https://keycloak.example.com/realms/test"
	m := newTestMiddleware(t, config, "test")
	req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
	req.Header.Set("X-Client-ID", "id")
	req.Header.Set("X-Client-Secret", "secret")
	_, _, hasBoth, hasAny := m.readClientCredentials(req)
	if !hasBoth || !hasAny {
		t.Fatal("expected both and any")
	}

	config.ClientIDHeader = ""
	config.ClientSecretHeader = ""
	m = newTestMiddleware(t, config, "test")
	_, _, hasBoth, hasAny = m.readClientCredentials(req)
	if hasBoth || hasAny {
		t.Fatal("expected no credentials when headers disabled")
	}
}

func TestServeHTTPMissingHeadersRequired(t *testing.T) {
	config := CreateConfig()
	config.KeycloakURL = "https://keycloak.example.com/realms/test"
	m := newTestMiddleware(t, config, "oidc-auth-middleware")

	req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	var payload map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if payload["message"] != "missing client credentials headers" {
		t.Fatalf("unexpected message: %s", payload["message"])
	}
	if payload["plugin"] != "oidc-auth-middleware" {
		t.Fatalf("unexpected plugin: %s", payload["plugin"])
	}
}

func TestServeHTTPMissingHeadersOptional(t *testing.T) {
	config := CreateConfig()
	config.KeycloakURL = "https://keycloak.example.com/realms/test"
	config.RequireClientCredentials = false
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	handler, err := New(context.Background(), next, config, "test")
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Fatal("expected next to be called")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestServeHTTPSuccess(t *testing.T) {
	config := CreateConfig()
	config.KeycloakURL = "https://keycloak.example.com/realms/test"
	m := newTestMiddleware(t, config, "test")
	m.httpClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"access_token":"abc123","token_type":"Bearer"}`)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	var seenAuth string
	var seenClientID string
	var seenClientSecret string
	m.next = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		seenClientID = r.Header.Get("X-Client-ID")
		seenClientSecret = r.Header.Get("X-Client-Secret")
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
	req.Header.Set("X-Client-ID", "id")
	req.Header.Set("X-Client-Secret", "secret")
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if seenAuth != "Bearer abc123" {
		t.Fatalf("unexpected auth header: %s", seenAuth)
	}
	if seenClientID != "" || seenClientSecret != "" {
		t.Fatalf("client headers should be removed")
	}
}

func TestServeHTTPExchangeError(t *testing.T) {
	config := CreateConfig()
	config.KeycloakURL = "https://keycloak.example.com/realms/test"
	m := newTestMiddleware(t, config, "test")
	m.httpClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(strings.NewReader(`{"error":"invalid_client","error_description":"bad secret"}`)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
	req.Header.Set("X-Client-ID", "id")
	req.Header.Set("X-Client-Secret", "secret")
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	var payload map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if payload["message"] != "invalid_client: bad secret" {
		t.Fatalf("unexpected message: %s", payload["message"])
	}
}

func TestExchangeTokenErrors(t *testing.T) {
	config := CreateConfig()
	config.KeycloakURL = "http://[::1"
	m := newTestMiddleware(t, config, "test")
	if _, err := m.exchangeToken(context.Background(), "id", "secret"); err == nil {
		t.Fatal("expected request create error")
	}

	config.KeycloakURL = "https://keycloak.example.com/realms/test"
	m = newTestMiddleware(t, config, "test")
	m.httpClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("network error")
		}),
	}
	if _, err := m.exchangeToken(context.Background(), "id", "secret"); err == nil {
		t.Fatal("expected network error")
	}

	m.httpClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       errReadCloser{},
				Header:     make(http.Header),
			}, nil
		}),
	}
	if _, err := m.exchangeToken(context.Background(), "id", "secret"); err == nil {
		t.Fatal("expected read error")
	}

	m.httpClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`not-json`)),
				Header:     make(http.Header),
			}, nil
		}),
	}
	if _, err := m.exchangeToken(context.Background(), "id", "secret"); err == nil {
		t.Fatal("expected parse error")
	}

	m.httpClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"access_token":""}`)),
				Header:     make(http.Header),
			}, nil
		}),
	}
	if _, err := m.exchangeToken(context.Background(), "id", "secret"); err == nil {
		t.Fatal("expected empty token error")
	}
}
