package telegramclient

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func TestCallResultDecodesBoundedUpdatesWithoutReplay(t *testing.T) {
	for _, tc := range []struct {
		name, method, body string
		status             int
		wantError          bool
	}{
		{name: "result", method: "getUpdates", body: `{"ok":true,"result":[{"update_id":1}]}`, status: 200},
		{name: "larger update", method: "getUpdates", body: `{"ok":true,"result":[{"text":"` + strings.Repeat("x", 70<<10) + `"}]}`, status: 200},
		{name: "oversized update", method: "getUpdates", body: strings.Repeat("x", (1<<20)+1), status: 200, wantError: true},
		{name: "ordinary method cap", method: "sendMessage", body: `{"result":["` + strings.Repeat("x", 70<<10) + `"]}`, status: 200, wantError: true},
		{name: "malformed", method: "getUpdates", body: `{"result":[`, status: 200, wantError: true},
		{name: "missing result", method: "getUpdates", body: `{"ok":true}`, status: 200, wantError: true},
		{name: "wrong result type", method: "getUpdates", body: `{"result":123}`, status: 200, wantError: true},
		{name: "rate limited", method: "getUpdates", body: `{"parameters":{"retry_after":3}}`, status: 429, wantError: true},
		{name: "server failure", method: "getUpdates", body: `{}`, status: 503, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := New("test")
			calls := 0
			c.http.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return telegramResponse(tc.status, tc.body), nil
			})
			var updates []json.RawMessage
			err := c.callResult(t.Context(), tc.method, nil, &updates)
			if (err != nil) != tc.wantError || calls != 1 {
				t.Fatalf("error=%v wantError=%v calls=%d", err, tc.wantError, calls)
			}
			if !tc.wantError && len(updates) != 1 {
				t.Fatalf("decoded %d updates", len(updates))
			}
		})
	}
}

func TestLongPollDoesNotHoldOutboundGate(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := New("test")
		start := c.pace.clock.Now()
		c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if strings.HasSuffix(r.URL.Path, "/getUpdates") {
				timer := time.NewTimer(5 * time.Second)
				defer timer.Stop()
				select {
				case <-timer.C:
				case <-r.Context().Done():
					return nil, r.Context().Err()
				}
				return telegramResponse(200, `{"ok":true,"result":[]}`), nil
			}
			return telegramResponse(200, `{"ok":true}`), nil
		})
		done := make(chan error, 1)
		go func() {
			var updates []json.RawMessage
			done <- c.callResult(t.Context(), "getUpdates", map[string]any{"timeout": 5, "limit": 1}, &updates)
		}()
		synctest.Wait()
		if err := c.SendMessage(t.Context(), 1, "reply", nil); err != nil {
			t.Fatal(err)
		}
		if elapsed := c.pace.clock.Now().Sub(start); elapsed != callSpacing {
			t.Fatalf("reply waited on long poll: %s", elapsed)
		}
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	})
}

type failedResponseBody struct{}

func (failedResponseBody) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (failedResponseBody) Close() error             { return nil }

func TestCallResultReturnsBodyReadFailure(t *testing.T) {
	c := New("test")
	calls := 0
	c.http.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Body: failedResponseBody{}, Header: make(http.Header)}, nil
	})
	var updates []json.RawMessage
	if err := c.callResult(t.Context(), "getUpdates", nil, &updates); err == nil || calls != 1 {
		t.Fatalf("error=%v calls=%d", err, calls)
	}
}
