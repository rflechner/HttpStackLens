# Function expressions in .http variables

The Go parser accepts literal variables and function expressions:

```http
@baseUrl = https://example.com
@token = oauth(
  clientId: "demo",
  clientSecret: demo-secret,
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
There is no implicit evaluation, caching, interpolation or builtin OAuth handler.
This parser API is not yet connected to the Composer's request execution or its
separate UI file editor parser.

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
