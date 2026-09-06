# Local OAuth test environment

English | [Français](README.fr.md)

Keycloak, a Go API, and a small browser application for developing OAuth support
in the Composer. No Composer changes are needed to send the requests in
`requests.http` manually.

## Getting started

Prerequisites: Docker with Compose v2 and a running Docker engine. From this folder:

```sh
docker compose up --build --wait --wait-timeout 180
```

From the repository root, add `-f tests/integration/oauth/compose.yaml` to the
Compose commands. The first run downloads Keycloak and the Go image.
The API health check verifies that the realm and introspection client are ready.

| Service | Address / demo credentials |
| --- | --- |
| Application and API | http://localhost:18081 |
| Keycloak console | http://localhost:18080/admin — `admin` / `admin-demo-only` |
| User | `alice` / `alice-demo-password` |
| Realm | `httpstacklens` |
| Machine client | `composer-service` / `composer-demo-secret` |
| Public browser client | `demo-app`, no secret, PKCE S256 required |
| API introspection client | `demo-api` / `demo-api-secret` |

Everything is disposable: there is no external database or persistent data volume.
The passwords are public fixtures intended only for this local test environment.
Ports are published on the loopback interface only. The environment uses local
HTTP; it does not yet cover HTTPS certificates or MITM.

```sh
docker compose down
# Also recreate the realm after changing the JSON:
docker compose up --build --force-recreate --wait --wait-timeout 180
```

Keycloak skips importing a realm that already exists. To start from scratch,
run `down`, then `up`. See the
[Keycloak import documentation](https://www.keycloak.org/server/importExport).

## Trying the application

Open **http://localhost:18081** (use `localhost`, not `127.0.0.1`, to match the
registered redirect URI). Choose to sign in with or without `demo:read`, then
sign in as Alice. The application uses Authorization Code with PKCE S256 and
`state` validation.

- `/public`: `200`, no authentication required.
- `/protected`: `200` with a valid access token, otherwise `401`.
- `/scoped`: `200` with `demo:read`, or `403` with a valid token without that scope.
- “Refresh token”: uses the refresh token from the browser flow.
- Access tokens expire after 120 seconds. Wait, then call the API to observe
  a `401` before refreshing the token.

Tokens stay in JavaScript memory and are not displayed. Only the PKCE verifier
and state are temporarily stored in `sessionStorage` to survive the redirect.
Reloading the page clears the tokens but does not end the Keycloak SSO session.
The implicit and password grant flows are disabled.

## Trying the Composer

Copy `requests.http` into the folder configured by `http_files.folder`, or paste
the requests into the Composer. Send the Client Credentials request, then replace
`PASTE_ACCESS_TOKEN` with the `access_token` value from the response.
This example does not use automatic variable substitution.

The machine client requests `scope=demo:read` to obtain the permission.
Without that parameter, `/protected` succeeds and `/scoped` returns `403`.
Client Credentials does not provide a refresh token: request a new access token.
Modified `.http` files may contain the copied token; do not commit them.
The Composer and traffic capture may expose this test data.

OAuth endpoints:

```text
Issuer:        http://localhost:18080/realms/httpstacklens
Discovery:     http://localhost:18080/realms/httpstacklens/.well-known/openid-configuration
Authorization: http://localhost:18080/realms/httpstacklens/protocol/openid-connect/auth
Token:         http://localhost:18080/realms/httpstacklens/protocol/openid-connect/token
```

The only configured redirect URI is `http://localhost:18081/`, for the demo
application. The future Composer callback must be registered in a separate
public client when implementing that feature.

If an upstream proxy is configured in HttpStackLens, include `localhost` and
`127.0.0.1` in `no_proxy` to reach these local fixtures.

## Validation

Go tests without Docker, from this folder (independent module, no dependencies):

```sh
cd api
go test ./...
```

This module's tests are not included in `go test ./...` at the repository root.

Use the Composer requests and demo application described above to check the
OAuth flows manually against Keycloak.

## Token validation and Docker networking

The API delegates cryptographic validation and token status checks to Keycloak
introspection using its confidential client. It then requires `active=true`,
the correct issuer, the `demo-api` audience, a future expiration time, and the
exact scope for `/scoped`. Simply decoding a JWT is not treated as proof of
authenticity. If Keycloak is unavailable, the API returns `503` and denies access.

The public issuer remains `http://localhost:18080/realms/httpstacklens` thanks to
`KC_HOSTNAME`. The API calls introspection at `http://keycloak:8080` from Docker:
this internal address never replaces the expected issuer. See the
[Keycloak hostname documentation](https://www.keycloak.org/server/hostname).

Both calling clients explicitly receive the `demo-api` audience through a mapper.
`demo:read` is an optional demo scope without a business role model.
The introspection client and its secret are hardcoded only in this test API.
The API does not log tokens or request bodies.
