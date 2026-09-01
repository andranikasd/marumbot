package app

import (
	"context"
	"strings"
	"testing"
)

type menuUsersFake struct {
	users []MenuUser
}

func (f menuUsersFake) MenuUsers(_ context.Context, after string, limit int32) ([]MenuUser, error) {
	start := 0
	for start < len(f.users) && f.users[start].ID <= after {
		start++
	}
	end := min(start+int(limit), len(f.users))
	return f.users[start:end], nil
}

type menuChatsFake struct{}

func (menuChatsFake) ChatID(_ context.Context, userID string) (int64, error) {
	return int64(len(userID)), nil
}

type menuSenderFake struct {
	buttons []string
}

func (f *menuSenderFake) SendMessage(context.Context, int64, string, any) error { return nil }
func (f *menuSenderFake) SendChatAction(context.Context, int64, string) error   { return nil }
func (f *menuSenderFake) SetChatMenuButtonFor(_ context.Context, _ int64, text, url string) error {
	f.buttons = append(f.buttons, text+"|"+url)
	return nil
}

func TestRefreshMenuButtonsPagesEveryAccount(t *testing.T) {
	users := make([]MenuUser, 205)
	for i := range users {
		users[i] = MenuUser{ID: string(rune(0x1000 + i)), Locale: "en"}
	}
	sender := &menuSenderFake{}
	w := &Worker{Chats: menuChatsFake{}, Send: sender, MiniApp: "https://example.test/app/", AppVersion: "v2"}

	n, err := w.RefreshMenuButtons(context.Background(), menuUsersFake{users: users})
	if err != nil {
		t.Fatal(err)
	}
	if n != len(users) || len(sender.buttons) != len(users) {
		t.Fatalf("refreshed=%d buttons=%d want=%d", n, len(sender.buttons), len(users))
	}
	for _, button := range sender.buttons {
		if want := "?v=v2"; !strings.Contains(button, want) {
			t.Fatalf("button %q does not contain %q", button, want)
		}
	}
}
