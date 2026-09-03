package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/andranikasd/marumbot/internal/i18n"
)

type workerLocaleFake struct {
	UserStore
	locale string
}

func (f *workerLocaleFake) Locale(context.Context, string) (string, string, error) {
	return f.locale, "UTC", nil
}

func (f *workerLocaleFake) SetLocale(_ context.Context, _, locale string) error {
	f.locale = locale
	return nil
}

type workerSenderFake struct {
	menus     []string
	messages  int
	typing    int
	typingErr error
	menuErr   error
	send      func(context.Context) error
}

func (f *workerSenderFake) SendChatAction(context.Context, int64, string) error {
	f.typing++
	return f.typingErr
}

type workerEmptyLoans struct{ LoanReader }

func (workerEmptyLoans) LoansForUser(context.Context, string, int32) ([]UserLoan, error) {
	return nil, nil
}

func (f *workerSenderFake) SetChatMenuButtonFor(_ context.Context, _ int64, label, url string) error {
	f.menus = append(f.menus, label+"|"+url)
	return f.menuErr
}

func (f *workerSenderFake) SendMessage(ctx context.Context, _ int64, _ string, _ any) error {
	f.messages++
	if f.send != nil {
		return f.send(ctx)
	}
	return nil
}

func senderWorker(send *workerSenderFake) *Worker {
	return &Worker{
		Users: &workerLocaleFake{locale: "hy"}, Send: send,
		Chats: menuChatsFake{}, Loans: workerEmptyLoans{}, Clock: fixedPaidClock{},
		Log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		MiniApp: "https://example.test/app/", AppVersion: "v2",
	}
}

func TestWorkerRefreshesChatMenuOnlyForStartAndLanguageChange(t *testing.T) {
	for _, tc := range []struct {
		name, kind, arg, data, text string
		wantMenu                    i18n.Locale
		wantTyping                  int
	}{
		{name: "start", kind: KindStart, wantMenu: i18n.HY},
		{name: "help", kind: KindHelp},
		{name: "add", kind: KindAdd},
		{name: "language prompt", kind: KindLanguage},
		{name: "language command", kind: KindLanguage, arg: "en", wantMenu: i18n.EN},
		{name: "language callback", kind: KindCallback, data: "lang:en", wantMenu: i18n.EN},
		{name: "reply keyboard", kind: KindText, text: i18n.Button(i18n.EN, KindAdd)},
		{name: "loans", kind: KindLoans, wantTyping: 1},
		{name: "advice", kind: KindAdvice, wantTyping: 1},
		{name: "loans keyboard", kind: KindText, text: i18n.Button(i18n.EN, KindLoans), wantTyping: 1},
		{name: "advice keyboard", kind: KindText, text: i18n.Button(i18n.EN, KindAdvice), wantTyping: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sender := &workerSenderFake{}
			w := senderWorker(sender)
			payload, err := json.Marshal(textPayload{Arg: tc.arg, Data: tc.data, Text: tc.text})
			if err != nil {
				t.Fatal(err)
			}
			if err := w.apply(t.Context(), InboundCommand{UserID: "user", Kind: tc.kind, Payload: payload}); err != nil {
				t.Fatal(err)
			}
			if sender.messages != 1 {
				t.Fatalf("replies=%d", sender.messages)
			}
			if sender.typing != tc.wantTyping {
				t.Fatalf("typing requests=%d want=%d", sender.typing, tc.wantTyping)
			}
			if tc.wantMenu == "" {
				if len(sender.menus) != 0 {
					t.Fatalf("ordinary command refreshed menu: %v", sender.menus)
				}
			} else {
				want := i18n.DashboardButton(tc.wantMenu) + "|" + w.miniURL("")
				if len(sender.menus) != 1 || sender.menus[0] != want {
					t.Fatalf("menus=%v want one %s", sender.menus, want)
				}
			}
		})
	}
}

func TestWorkerTypingFailureDoesNotBlockLoanReply(t *testing.T) {
	sender := &workerSenderFake{typingErr: errors.New("typing unavailable")}
	w := senderWorker(sender)
	if err := w.apply(t.Context(), InboundCommand{UserID: "user", Kind: KindLoans}); err != nil || sender.messages != 1 || sender.typing != 1 {
		t.Fatalf("typing failure blocked reply: error=%v replies=%d typing=%d", err, sender.messages, sender.typing)
	}
}

func TestWorkerMenuSetupRemainsBestEffort(t *testing.T) {
	sender := &workerSenderFake{menuErr: errors.New("menu unavailable")}
	w := senderWorker(sender)
	if err := w.apply(t.Context(), InboundCommand{UserID: "user", Kind: KindStart}); err != nil || sender.messages != 1 {
		t.Fatalf("start was blocked by menu setup: %v, replies=%d", err, sender.messages)
	}
	w.MiniApp = ""
	if err := w.apply(t.Context(), InboundCommand{UserID: "user", Kind: KindStart}); err != nil || len(sender.menus) != 1 {
		t.Fatalf("missing MiniApp should skip setup: %v, menus=%v", err, sender.menus)
	}
}

type workerFailureFake struct {
	InboxStore
	retryAt time.Time
	failed  int
	ctxErr  error
}

func (f *workerFailureFake) Fail(ctx context.Context, _, _, _ string, at time.Time, _ bool) error {
	f.failed++
	f.retryAt = at
	f.ctxErr = ctx.Err()
	return nil
}

type workerSlowError struct{ wait time.Duration }

func (e workerSlowError) Error() string             { return "rate limited" }
func (e workerSlowError) RetryAfter() time.Duration { return e.wait }

func TestWorkerPreservesRetryAfterSchedulingWithoutImmediateReplay(t *testing.T) {
	for _, wait := range []time.Duration{time.Second, 45 * time.Second} {
		sender := &workerSenderFake{send: func(context.Context) error { return workerSlowError{wait: wait} }}
		w := senderWorker(sender)
		inbox := &workerFailureFake{}
		w.Inbox = inbox
		w.handle(t.Context(), Lease{Command: InboundCommand{ID: "command", UserID: "user", Kind: KindAdd, Attempts: 1}})
		want := w.Clock.Now().Add(max(RetryAfter(1), wait))
		if inbox.failed != 1 || !inbox.retryAt.Equal(want) || sender.messages != 1 {
			t.Fatalf("failed=%d retry=%s want=%s sends=%d", inbox.failed, inbox.retryAt, want, sender.messages)
		}
	}
}

func TestWorkerPacedSendCannotOutliveLease(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sender := &workerSenderFake{send: func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		}}
		w := senderWorker(sender)
		inbox := &workerFailureFake{}
		w.Inbox = inbox
		w.handle(t.Context(), Lease{
			Until:   w.Clock.Now().Add(time.Second),
			Command: InboundCommand{ID: "command", UserID: "user", Kind: KindAdd, Attempts: 1},
		})
		if inbox.failed != 1 || inbox.ctxErr != nil || sender.messages != 1 {
			t.Fatalf("lease timeout did not settle with live parent: failed=%d context=%v sends=%d", inbox.failed, inbox.ctxErr, sender.messages)
		}
	})
}

func TestWorkerLanguageSetupUsesNewLocale(t *testing.T) {
	sender := &workerSenderFake{}
	w := senderWorker(sender)
	if err := w.setLanguage(t.Context(), "user", 1, i18n.EN); err != nil {
		t.Fatal(err)
	}
	if w.Users.(*workerLocaleFake).locale != "en" || !strings.HasPrefix(sender.menus[0], i18n.DashboardButton(i18n.EN)+"|") {
		t.Fatal("language setup used the previous locale")
	}
}
