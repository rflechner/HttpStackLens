package composer

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
)

var variableReference = regexp.MustCompile(`\{\{\s*([\w.-]+)\s*\}\}`)

// Scope holds evaluated variables for one file in memory. Its owner serializes
// Evaluate and Reset. Only declarations, not request edits, invalidate the cache.
type Scope struct {
	signature string
	values    map[string]string
}

func (s *Scope) Reset() { s.signature = ""; s.values = nil }

func declarationSignature(vars []FileVariable) string {
	type declaration struct{ Name, Value string }
	declarations := make([]declaration, len(vars))
	for i, v := range vars {
		declarations[i] = declaration{v.Name.Text, v.Value.Text}
	}
	encoded, _ := json.Marshal(declarations)
	return string(encoded)
}

func (s *Scope) Ready(vars []FileVariable) bool {
	return s.values != nil && s.signature == declarationSignature(vars)
}

// Evaluate resolves declarations in file order. Failed initialization is never
// cached, and resolved secrets are never written back into the parsed file.
func (s *Scope) Evaluate(ctx context.Context, vars []FileVariable, handler FunctionHandler) (map[string]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	signature := declarationSignature(vars)
	if !s.Ready(vars) {
		s.Reset()
		values := make(map[string]string, len(vars))
		for _, v := range vars {
			if _, exists := values[v.Name.Text]; exists {
				return nil, fmt.Errorf("duplicate variable declaration")
			}
			resolve := func(text string) (string, error) {
				missing := false
				result := variableReference.ReplaceAllStringFunc(text, func(match string) string {
					value, ok := values[variableReference.FindStringSubmatch(match)[1]]
					if !ok {
						missing = true
					}
					return value
				})
				if missing {
					return "", fmt.Errorf("variable references must refer to an earlier declaration")
				}
				return result, nil
			}
			evaluated := v
			if v.Call != nil {
				call := &FunctionCall{Name: v.Call.Name, Parameters: map[string]string{}}
				for key, value := range v.Call.Parameters {
					resolved, err := resolve(value)
					if err != nil {
						return nil, err
					}
					call.Parameters[key] = resolved
				}
				evaluated.Call = call
			} else {
				resolved, err := resolve(v.Value.Text)
				if err != nil {
					return nil, err
				}
				evaluated.Value.Text = resolved
			}
			value, err := evaluated.Evaluate(ctx, handler)
			if err != nil {
				return nil, err
			}
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			values[v.Name.Text] = value
		}
		s.signature, s.values = signature, values
	}
	result := make(map[string]string, len(s.values))
	for key, value := range s.values {
		result[key] = value
	}
	return result, nil
}
