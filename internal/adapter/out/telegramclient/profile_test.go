package telegramclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSetLocalizedProfileFields(t *testing.T) {
	tests := []struct {
		name, method, field, lang, value string
		call                             func(*Client) error
	}{
		{
			name: "name", method: "setMyName", field: "name", lang: "en", value: "Marum",
			call: func(c *Client) error { return c.SetMyName(context.Background(), "en", "Marum") },
		},
		{
			name: "short description", method: "setMyShortDescription", field: "short_description", value: "short",
			call: func(c *Client) error { return c.SetMyShortDescription(context.Background(), "", "short") },
		},
		{
			name: "description", method: "setMyDescription", field: "description", lang: "hy", value: "long",
			call: func(c *Client) error { return c.SetMyDescription(context.Background(), "hy", "long") },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := New("token").WithBase("https://telegram.test")
			client.http = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path != "/bottoken/"+tt.method {
					t.Errorf("path = %q", r.URL.Path)
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if body[tt.field] != tt.value {
					t.Errorf("%s = %q, want %q", tt.field, body[tt.field], tt.value)
				}
				if got, ok := body["language_code"]; (tt.lang == "" && ok) || (tt.lang != "" && got != tt.lang) {
					t.Errorf("language_code = %q, present=%v", got, ok)
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Body:       io.NopCloser(strings.NewReader(`{"ok":true,"result":true}`)),
					Header:     make(http.Header),
				}, nil
			})}

			if err := tt.call(client); err != nil {
				t.Fatal(err)
			}
		})
	}
}
