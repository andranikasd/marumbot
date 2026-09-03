package telegramclient

import (
	"context"
	"net/http"
	"testing"
	"testing/synctest"
	"time"
)

func TestObserverCountsReceived429OnlyAndIncludesWait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var observations []CallObservation
		c := New("secret").WithObserver(func(_ context.Context, o CallObservation) {
			observations = append(observations, o)
		})
		calls := 0
		c.http.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			calls++
			if calls == 1 {
				return telegramResponse(429, `{"parameters":{"retry_after":2}}`), nil
			}
			return telegramResponse(200, `{"ok":true}`), nil
		})
		_ = c.SendMessage(t.Context(), 1, "private", nil)
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		_ = c.SendMessage(ctx, 2, "not sent", nil)
		_ = c.call(t.Context(), "unbounded-method-input", nil)
		want := []CallObservation{
			{Method: "sendMessage", Outcome: "rate_limited", RateLimited: true},
			{Method: "sendMessage", Outcome: "canceled", Duration: time.Second},
			{Method: "other", Outcome: "success", Duration: time.Second},
		}
		if len(observations) != len(want) || calls != 2 {
			t.Fatalf("observations=%d calls=%d", len(observations), calls)
		}
		for i := range want {
			if observations[i] != want[i] {
				t.Errorf("observation %d: %+v, want %+v", i, observations[i], want[i])
			}
		}
	})
}
