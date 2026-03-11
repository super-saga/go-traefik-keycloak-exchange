package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	middleware "github.com/super-saga/go-traefik-keycloak-exchange"
)

func main() {
	config := middleware.CreateConfig()
	if value := strings.TrimSpace(os.Getenv("KEYCLOAK_URL")); value != "" {
		config.KeycloakURL = value
	}
	if value := strings.TrimSpace(os.Getenv("CLIENT_ID_HEADER")); value != "" {
		config.ClientIDHeader = value
	}
	if value := strings.TrimSpace(os.Getenv("CLIENT_SECRET_HEADER")); value != "" {
		config.ClientSecretHeader = value
	}
	if value := strings.TrimSpace(os.Getenv("REQUIRE_CLIENT_CREDENTIALS")); value != "" {
		config.RequireClientCredentials = value == "true" || value == "1"
	}
	if value := strings.TrimSpace(os.Getenv("TOKEN_REQUEST_TIMEOUT_SECONDS")); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			config.TokenRequestTimeoutSeconds = parsed
		}
	}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"message": "OK - middleware passed",
			"token":   r.Header.Get("Authorization"),
		})
	})

	handler, err := middleware.New(context.Background(), next, config, "oidc-auth-middleware")
	if err != nil {
		log.Fatalf("middleware init failed: %v", err)
	}

	log.Println("middleware debug server started on :8080")
	if err := http.ListenAndServe(":8080", handler); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}
