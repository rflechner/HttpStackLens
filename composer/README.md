# Function expressions in .http variables

The Go parser accepts literal variables and function expressions:

```http
@baseUrl = https://example.com
@token = oauth(
  issuer: "http://localhost:18080/realms/httpstacklens",
  grantType: "client_credentials",
  clientId: "composer-service",
  clientSecret: composer-demo-secret,
  scope: "demo:read",
)

### Protected request
GET {{baseUrl}}/protected
Authorization: Bearer {{token}}
```

`ParseHttpFile` preserves the expression in `FileVariable.Value` with its source
positions and exposes `FileVariable.Call` as:

```go
type FunctionCall struct {
    Name       string
    Parameters map[string]string
}
```

Parsing and highlighting never execute functions. A Go caller explicitly
evaluates a variable with its own handler:

```go
file := composer.ParseHttpFile(source)
if len(file.Issues) != 0 {
    // Report syntax errors before evaluating anything.
    return
}

handler := func(ctx context.Context, name string, parameters map[string]string) (string, error) {
    switch name {
    case "oauth":
        // Application-owned implementation; validate the required parameters here.
        return acquireToken(ctx, parameters["clientId"], parameters["clientSecret"])
    default:
        return "", fmt.Errorf("unsupported function")
    }
}
token, err := file.Variables[1].Evaluate(ctx, handler)
```

Literal evaluation returns the original value. Function evaluation passes the
context, function name and a copy of the parameter dictionary to the handler.
The parser itself performs no implicit evaluation. The Web UI connects its send
actions to `Scope.Evaluate` and an OAuth handler. `Scope` resolves references to
earlier declarations, caches successful initialization and can be reset. Its
owner must serialize evaluation and reset operations.

Arguments must be named. All values are strings: bare `true` and `12` remain
strings. Single- and double-quoted values support Go string escapes (including escaped
matching quotes); use quotes for spaces,
commas or parentheses. Empty strings are allowed. Calls can span lines and may
end with a trailing comma. Duplicate names, positional arguments, nested calls
(including `env(...)`) and text after a call are rejected. Function and parameter
identifiers begin with a letter or underscore, followed by letters, digits,
underscores or hyphens.

Parameter values are not included in parser diagnostics. Handlers must also avoid
exposing secrets in errors. Only invoke handlers when the user requests execution;
the file may name any function, and the handler decides which ones are allowed.

## OAuth in the Composer

```http
@issuer = http://localhost:18080/realms/httpstacklens
@token = oauth(
  issuer: '{{issuer}}',
  clientId: 'composer-browser',
  scope: 'demo:read',
  prompt: 'login',
)

### Protected API
GET http://localhost:18081/protected
Authorization: Bearer {{token}}
```

`oauth` returns the access token only; add `Bearer` in the header yourself.
Supported parameters:

| Parameter | Meaning |
| --- | --- |
| `issuer` | Required OAuth/OIDC issuer; endpoints are discovered from its `.well-known/openid-configuration`. |
| `clientId` | Required registered client ID. |
| `grantType` | `authorization_code` (default, PKCE S256 popup) or `client_credentials` (no popup). |
| `clientSecret` | Required for client credentials; optional for a confidential authorization-code client. Uses `client_secret_post`. |
| `scope` | Optional space-separated OAuth scopes. |
| `prompt` | Optional authorization prompt, for example `login` to show the login form even with an existing SSO session. |

Register `<Web UI origin>/api/composer/oauth/callback` as an exact redirect URI.
The local fixture includes `composer-browser` for `localhost:9000` and
`127.0.0.1:9000`. Other UI ports must be registered in the provider.
If the WebView or browser cannot create a popup, the backend opens the login
in the system browser. Use **Cancel** in the Composer to cancel that login;
closing an external browser tab cannot be detected. Closing a managed popup cancels initialization;
the request is not sent if initialization fails. Login times out after five minutes.

Every send entry point initializes all file declarations before sending. Values
stay in memory per file, without replacing expressions in the editor or on disk.
**Reset scope**, changing declarations, reloading the UI, or deleting the file
clears that cache. Editing a request keeps it. Known token expiry triggers a new
initialization on the next send; refresh tokens are not used. If the provider
omits `expires_in`, reset manually when its token expires or is revoked.
Resetting does not log out of the provider's SSO session or revoke tokens.

Discovery and token exchange run in the backend using the default Go HTTP
transport (including environment proxy settings), outside the captured proxy
pipeline. The configured HttpStackLens upstream proxy and MITM settings do not
apply to those exchanges. API requests still use the usual Composer pipeline
and their Authorization headers can appear in traffic inspection/capture.
OAuth URLs require HTTPS except for loopback fixtures. No passwords are collected
by HttpStackLens; the identity provider owns the login form.

Protocol references: [Keycloak discovery and endpoints](https://www.keycloak.org/securing-apps/oidc-layers)
and [OAuth security best practices](https://www.rfc-editor.org/info/rfc9700/).
