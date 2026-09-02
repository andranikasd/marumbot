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
	profile []string
}

func (f *menuSenderFake) SendMessage(context.Context, int64, string, any) error { return nil }
func (f *menuSenderFake) SendChatAction(context.Context, int64, string) error   { return nil }
func (f *menuSenderFake) SetChatMenuButtonFor(_ context.Context, _ int64, text, url string) error {
	f.buttons = append(f.buttons, text+"|"+url)
	return nil
}

func (f *menuSenderFake) SetMyCommands(context.Context, string, []BotCommand) error { return nil }
func (f *menuSenderFake) SetChatMenuButton(context.Context, string, string) error   { return nil }
func (f *menuSenderFake) SetMyName(_ context.Context, lang, value string) error {
	f.profile = append(f.profile, "name|"+lang+"|"+value)
	return nil
}

func (f *menuSenderFake) SetMyShortDescription(_ context.Context, lang, value string) error {
	f.profile = append(f.profile, "short|"+lang+"|"+value)
	return nil
}

func (f *menuSenderFake) SetMyDescription(_ context.Context, lang, value string) error {
	f.profile = append(f.profile, "description|"+lang+"|"+value)
	return nil
}

func TestPublishMenusPublishesLocalizedProfileAndDefault(t *testing.T) {
	sender := &menuSenderFake{}
	if err := PublishMenus(context.Background(), sender, ""); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"name|hy|Մարում", "short|hy|", "description|hy|",
		"name||Մարում", "short||", "description||",
		"name|en|Marum", "short|en|", "description|en|",
	}
	if len(sender.profile) != len(want) {
		t.Fatalf("profile calls = %d, want %d: %v", len(sender.profile), len(want), sender.profile)
	}
	for i, prefix := range want {
		if !strings.HasPrefix(sender.profile[i], prefix) {
			t.Errorf("profile call %d = %q, want prefix %q", i, sender.profile[i], prefix)
		}
	}
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
