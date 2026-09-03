package telegramclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func telegramResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status, Status: http.StatusText(status),
		Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header),
	}
}

func TestOutboundPacingAcrossMethodsAndChats(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := New("test")
		start := c.pace.clock.Now()
		var sent []time.Duration
		c.http.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			sent = append(sent, c.pace.clock.Now().Sub(start))
			return telegramResponse(200, `{"ok":true}`), nil
		})
		calls := []func() error{
			func() error { return c.SendMessage(t.Context(), 1, "first", nil) },
			func() error { return c.AnswerCallbackQuery(t.Context(), "callback") },
			func() error { return c.SendMessage(t.Context(), 1, "second", nil) },
			func() error { return c.SendMessage(t.Context(), 2, "other chat", nil) },
			func() error { return c.SetChatMenuButtonFor(t.Context(), 2, "Open", "https://example.test") },
			func() error { return c.SendChatAction(t.Context(), 2, "typing") },
		}
		for _, call := range calls {
			if err := call(); err != nil {
				t.Fatal(err)
			}
		}
		want := []time.Duration{0, 40 * time.Millisecond, time.Second, 1040 * time.Millisecond, 1080 * time.Millisecond, 1120 * time.Millisecond}
		for i := range want {
			if sent[i] != want[i] {
				t.Errorf("call %d at %s, want %s", i, sent[i], want[i])
			}
		}
	})
}

func TestWaitingChatDoesNotBlockOtherChatsAndCancellationConsumesNoSlot(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := New("test")
		start := c.pace.clock.Now()
		calls := 0
		c.http.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			calls++
			return telegramResponse(200, `{"ok":true}`), nil
		})
		if err := c.SendMessage(t.Context(), 1, "first", nil); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() { done <- c.SendMessage(ctx, 1, "cancelled", nil) }()
		synctest.Wait()
		if err := c.SendMessage(t.Context(), 2, "other chat", nil); err != nil {
			t.Fatal(err)
		}
		if elapsed := c.pace.clock.Now().Sub(start); elapsed != callSpacing {
			t.Fatalf("other chat waited %s", elapsed)
		}
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled send: %v", err)
		}
		if err := c.SendMessage(t.Context(), 1, "second", nil); err != nil {
			t.Fatal(err)
		}
		if elapsed := c.pace.clock.Now().Sub(start); elapsed != time.Second || calls != 3 {
			t.Fatalf("cancelled call consumed a slot: elapsed=%s calls=%d", elapsed, calls)
		}
	})
}

func Test429DelaysAlreadyWaitingCallsWithoutReplayingSend(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := New("test")
		start := c.pace.clock.Now()
		release := make(chan struct{})
		calls := 0
		c.http.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			calls++
			if calls == 1 {
				<-release
				return telegramResponse(429, `{"parameters":{"retry_after":3}}`), nil
			}
			if c.pace.clock.Now().Sub(start) < 3*time.Second {
				t.Error("request bypassed cooldown")
			}
			return telegramResponse(200, `{"ok":true}`), nil
		})
		first, second := make(chan error, 1), make(chan error, 1)
		go func() { first <- c.SendMessage(t.Context(), 1, "first", nil) }()
		synctest.Wait()
		go func() { second <- c.AnswerCallbackQuery(t.Context(), "callback") }()
		synctest.Wait()
		close(release)
		var limited *TooManyError
		if err := <-first; !errors.As(err, &limited) || limited.Wait != 3*time.Second || !errors.Is(err, ErrRetryable) {
			t.Fatalf("429 classification: %v", err)
		}
		// A shorter later cooldown cannot release an existing longer one.
		c.pace.cooldown(time.Second)
		if err := <-second; err != nil {
			t.Fatal(err)
		}
		if calls != 2 || c.pace.clock.Now().Sub(start) != 3*time.Second {
			t.Fatalf("unexpected replay or cooldown: calls=%d elapsed=%s", calls, c.pace.clock.Now().Sub(start))
		}
	})
}

func TestCooldownRespectsDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := New("test")
		calls := 0
		c.http.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			calls++
			return telegramResponse(429, `{"parameters":{"retry_after":60}}`), nil
		})
		_ = c.SendMessage(t.Context(), 1, "limited", nil)
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		if err := c.SendMessage(ctx, 2, "must not send", nil); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("deadline: %v", err)
		}
		if calls != 1 {
			t.Fatal("cancelled cooldown reached transport")
		}
	})
}

func TestTransportFailuresAreNeverAutomaticallyReplayed(t *testing.T) {
	for _, status := range []int{0, 503, 429} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			c := New("test")
			calls := 0
			c.http.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				if status == 0 {
					return nil, io.ErrUnexpectedEOF
				}
				return telegramResponse(status, `{}`), nil
			})
			err := c.SendMessage(t.Context(), 1, "unknown outcome", nil)
			if !errors.Is(err, ErrRetryable) || calls != 1 {
				t.Fatalf("error=%v calls=%d", err, calls)
			}
			if status == 429 {
				var limited *TooManyError
				if !errors.As(err, &limited) || limited.Wait != time.Second {
					t.Fatalf("missing retry_after fallback: %v", err)
				}
			}
		})
	}
}

func TestRetryAfterCannotOverflow(t *testing.T) {
	if got := retryAfter([]byte(`{"parameters":{"retry_after":9223372036854775807}}`)); got <= 0 {
		t.Fatalf("overflowed cooldown: %s", got)
	}
}
