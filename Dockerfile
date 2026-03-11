FROM alpine:3.20 AS plugin

WORKDIR /plugins-local/src/github.com/super-saga/go-traefik-keycloak-exchange
COPY . .

FROM traefik:v2.11
COPY --from=plugin /plugins-local /plugins-local
