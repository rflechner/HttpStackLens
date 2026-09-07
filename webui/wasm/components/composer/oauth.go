//go:build js && wasm

package composer

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"syscall/js"
	"time"

	httpfile "httpStackLens/composer"
	"httpStackLens/webui/wasm/dom"
)

type fileScope struct {
	values  httpfile.Scope
	expires time.Time
}

func (c *Composer) CancelLogin() {
	if c.cancelLogin != nil {
		c.cancelLogin()
	}
}

func (c *Composer) ResetScope() {
	if c.CurFile == nil || c.Sending {
		return
	}
	delete(c.scopes, c.CurFile.ID)
	c.StateHasChanged()
}

// Open the window inside the click handler, before any asynchronous discovery:
// opening it after a fetch would lose browser user activation and be blocked.
func (c *Composer) startScoped(file httpfile.HttpFile, build func([]KV) outgoing) {
	if c.Sending || c.CurFile == nil {
		return
	}
	if len(file.Issues) != 0 {
		c.Res = &Result{NotSent: true, Err: "Fix the .http syntax errors before sending."}
		c.StateHasChanged()
		return
	}
	if c.scopes == nil {
		c.scopes = map[string]*fileScope{}
	}
	scope := c.scopes[c.CurFile.ID]
	if scope == nil {
		scope = &fileScope{}
		c.scopes[c.CurFile.ID] = scope
	}
	if !scope.expires.IsZero() && !time.Now().Before(scope.expires) {
		scope.values.Reset()
	}
	ready := scope.values.Ready(file.Variables)
	var popup js.Value
	if !ready {
		scope.expires = time.Time{}
		for _, v := range file.Variables {
			if v.Call != nil && v.Call.Name == "oauth" && v.Call.Parameters["grantType"] != "client_credentials" {
				popup = js.Global().Call("open", "about:blank", "_blank", "popup,width=520,height=720")
				if popup.Truthy() {
					popup.Set("opener", js.Null())
				}
				break
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	c.cancelLogin = cancel
	c.LoginPending = !ready
	fileID := c.CurFile.ID
	c.Sending = true
	c.Res = nil
	c.StateHasChanged()
	go func() {
		var result *Result
		defer func() {
			if popup.Truthy() {
				popup.Call("close")
			}
			if c.CurFile != nil && c.CurFile.ID == fileID {
				c.Res = result
			}
			c.Sending = false
			c.LoginPending = false
			c.cancelLogin = nil
			c.resp.Tab = "body"
			c.StateHasChanged()
		}()
		defer cancel()
		values, err := scope.values.Evaluate(ctx, file.Variables, func(ctx context.Context, name string, params map[string]string) (string, error) {
			if name != "oauth" {
				return "", errors.New("unsupported function; available function: oauth")
			}
			token, expires, err := evaluateOAuth(ctx, params, popup)
			if err != nil {
				return "", err
			}
			if !expires.IsZero() && (scope.expires.IsZero() || expires.Before(scope.expires)) {
				scope.expires = expires
			}
			return token, nil
		})
		if err != nil {
			result = &Result{NotSent: true, Err: err.Error()}
			return
		}
		vars := make([]KV, 0, len(values))
		for name, value := range values {
			vars = append(vars, KV{Key: name, Value: value, On: true})
		}
		c.LoginPending = false
		c.StateHasChanged()
		result = exchange(build(vars))
	}()
}

type oauthReply struct {
	ID               string `json:"id"`
	AuthorizationURL string `json:"authorizationUrl"`
	AccessToken      string `json:"accessToken"`
	ExpiresIn        int    `json:"expiresIn"`
	Error            string `json:"error"`
}

func evaluateOAuth(ctx context.Context, params map[string]string, popup js.Value) (string, time.Time, error) {
	payload, _ := json.Marshal(params)
	res, err := dom.Fetch("POST", "/api/composer/oauth", map[string]string{"Content-Type": "application/json"}, string(payload))
	if err != nil {
		return "", time.Time{}, errors.New("could not reach the OAuth service")
	}
	if res.Status != 200 {
		return "", time.Time{}, errors.New(strings.TrimSpace(res.Body))
	}
	var reply oauthReply
	if json.Unmarshal([]byte(res.Body), &reply) != nil {
		return "", time.Time{}, errors.New("invalid OAuth response")
	}
	if reply.ID != "" {
		headers := map[string]string{"X-OAuth-ID": reply.ID}
		defer func() { _, _ = dom.Fetch("DELETE", "/api/composer/oauth/result", headers, "") }()
		if popup.Truthy() {
			if popup.Get("closed").Bool() {
				return "", time.Time{}, errors.New("OAuth login window was closed; send again to retry")
			}
			popup.Get("location").Set("href", reply.AuthorizationURL)
		} else {
			opened, err := dom.Fetch("POST", "/api/composer/oauth/open", headers, "")
			if err != nil || opened.Status != 204 {
				return "", time.Time{}, errors.New("Could not open the system browser for OAuth login.")
			}
		}
		for {
			select {
			case <-ctx.Done():
				if errors.Is(ctx.Err(), context.Canceled) {
					return "", time.Time{}, errors.New("OAuth login cancelled")
				}
				return "", time.Time{}, errors.New("OAuth login timed out")
			case <-time.After(400 * time.Millisecond):
			}
			// Poll before checking closed: the callback may have completed just before
			// the user closed the window.
			res, err = dom.Fetch("POST", "/api/composer/oauth/result", headers, "")
			if err != nil {
				return "", time.Time{}, errors.New("could not read OAuth login result")
			}
			if res.Status == 200 {
				if json.Unmarshal([]byte(res.Body), &reply) != nil {
					return "", time.Time{}, errors.New("invalid OAuth response")
				}
				break
			}
			if res.Status != 202 {
				return "", time.Time{}, errors.New(strings.TrimSpace(res.Body))
			}
			if popup.Truthy() && popup.Get("closed").Bool() {
				return "", time.Time{}, errors.New("OAuth login cancelled")
			}
		}
	}
	if reply.Error != "" {
		return "", time.Time{}, errors.New(reply.Error)
	}
	if reply.AccessToken == "" {
		return "", time.Time{}, errors.New("OAuth returned no access token")
	}
	expires := time.Time{}
	if reply.ExpiresIn > 0 {
		expires = time.Now().Add(time.Duration(reply.ExpiresIn) * time.Second)
	}
	return reply.AccessToken, expires, nil
}
