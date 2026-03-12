package go_traefik_keycloak_exchange

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Config struct {
	KeycloakURL                string `json:"keycloakURL" toml:"keycloakURL" yaml:"keycloakURL"`
	ClientIDHeader             string `json:"clientIDHeader" toml:"clientIDHeader" yaml:"clientIDHeader"`
	ClientSecretHeader         string `json:"clientSecretHeader" toml:"clientSecretHeader" yaml:"clientSecretHeader"`
	RequireClientCredentials   bool   `json:"requireClientCredentials" toml:"requireClientCredentials" yaml:"requireClientCredentials"`
	TokenRequestTimeoutSeconds int    `json:"tokenRequestTimeoutSeconds" toml:"tokenRequestTimeoutSeconds" yaml:"tokenRequestTimeoutSeconds"`
}

func CreateConfig() *Config {
	return &Config{
		ClientIDHeader:             "X-Client-ID",
		ClientSecretHeader:         "X-Client-Secret",
		RequireClientCredentials:   true,
		TokenRequestTimeoutSeconds: 10,
	}
}

type Middleware struct {
	next       http.Handler
	name       string
	config     *Config
	httpClient *http.Client
}

func New(_ context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	if config == nil {
		return nil, errors.New("config is required")
	}
	if strings.TrimSpace(config.KeycloakURL) == "" {
		return nil, errors.New("keycloakURL is required")
	}

	timeout := time.Duration(config.TokenRequestTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	return &Middleware{
		next:   next,
		name:   name,
		config: config,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}, nil
}

func (m *Middleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	clientID, clientSecret, hasBoth, hasAny := m.readClientCredentials(r)
	if !hasBoth {
		if !hasAny && !m.config.RequireClientCredentials {
			m.next.ServeHTTP(w, r)
			return
		}
		m.respondUnauthorized(w, "missing client credentials headers")
		return
	}

	accessToken, err := m.exchangeToken(r.Context(), clientID, clientSecret)
	if err != nil {
		m.respondUnauthorized(w, err.Error())
		return
	}

	r.Header.Del(m.config.ClientIDHeader)
	r.Header.Del(m.config.ClientSecretHeader)
	r.Header.Set("Authorization", "Bearer "+accessToken)
	m.next.ServeHTTP(w, r)
}

func (m *Middleware) readClientCredentials(r *http.Request) (string, string, bool, bool) {
	clientIDHeader := strings.TrimSpace(m.config.ClientIDHeader)
	clientSecretHeader := strings.TrimSpace(m.config.ClientSecretHeader)
	if clientIDHeader == "" && clientSecretHeader == "" {
		return "", "", false, false
	}

	clientID := ""
	clientSecret := ""
	if clientIDHeader != "" {
		clientID = strings.TrimSpace(r.Header.Get(clientIDHeader))
	}
	if clientSecretHeader != "" {
		clientSecret = strings.TrimSpace(r.Header.Get(clientSecretHeader))
	}
	hasAny := clientID != "" || clientSecret != ""
	hasBoth := clientID != "" && clientSecret != ""
	return clientID, clientSecret, hasBoth, hasAny
}

type tokenResponse struct {
	AccessToken      string `json:"access_token"`
	TokenType        string `json:"token_type"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

func (m *Middleware) exchangeToken(ctx context.Context, clientID, clientSecret string) (string, error) {
	tokenEndpoint := strings.TrimRight(m.config.KeycloakURL, "/") + "/protocol/openid-connect/token"
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("token request create failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("token response read failed: %w", err)
	}

	var parsed tokenResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("token response parse failed: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := parsed.Error
		if message == "" {
			message = "token exchange rejected"
		}
		if parsed.ErrorDescription != "" {
			message = message + ": " + parsed.ErrorDescription
		}
		return "", errors.New(message)
	}

	if strings.TrimSpace(parsed.AccessToken) == "" {
		return "", errors.New("token exchange returned empty access_token")
	}

	return parsed.AccessToken, nil
}

func (m *Middleware) respondUnauthorized(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"message": message,
		"plugin":  m.name,
	})
}
