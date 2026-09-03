package app

import (
	"context"
	"errors"
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
	buttons          []string
	profile          []string
	globalURL        string
	commandLanguages []string
	profileErr       error
	commandErr       error
}

func (f *menuSenderFake) SendMessage(context.Context, int64, string, any) error { return nil }
func (f *menuSenderFake) SendChatAction(context.Context, int64, string) error   { return nil }
func (f *menuSenderFake) SetChatMenuButtonFor(_ context.Context, _ int64, text, url string) error {
	f.buttons = append(f.buttons, text+"|"+url)
	return nil
}

func (f *menuSenderFake) SetMyCommands(_ context.Context, lang string, _ []BotCommand) error {
	f.commandLanguages = append(f.commandLanguages, lang)
	return f.commandErr
}

func (f *menuSenderFake) SetChatMenuButton(_ context.Context, _, url string) error {
	f.globalURL = url
	return nil
}

func (f *menuSenderFake) SetMyName(_ context.Context, lang, value string) error {
	f.profile = append(f.profile, "name|"+lang+"|"+value)
	return f.profileErr
}

func (f *menuSenderFake) SetMyShortDescription(_ context.Context, lang, value string) error {
	f.profile = append(f.profile, "short|"+lang+"|"+value)
	return nil
}

func (f *menuSenderFake) SetMyDescription(_ context.Context, lang, value string) error {
	f.profile = append(f.profile, "description|"+lang+"|"+value)
	return nil
}

func TestPublishProfilePublishesLocalizedProfileAndDefault(t *testing.T) {
	sender := &menuSenderFake{}
	if err := PublishProfile(context.Background(), sender); err != nil {
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

func TestProfileRateLimitDoesNotBlockMenus(t *testing.T) {
	rateLimit := errors.New("profile rate limited")
	sender := &menuSenderFake{profileErr: rateLimit}
	ctx := context.Background()
	if err := PublishProfile(ctx, sender); !errors.Is(err, rateLimit) {
		t.Fatalf("profile error = %v", err)
	}
	var publication MenuPublication
	const launch = "https://example.test/app/?v=2.0.0"
	publication.Publish(ctx, sender, launch, nil)
	publication.Publish(ctx, sender, launch, nil)
	if sender.globalURL != launch || len(sender.commandLanguages) != 3 {
		t.Fatalf("launch = %q, commands = %v", sender.globalURL, sender.commandLanguages)
	}
	if len(sender.profile) != 1 {
		t.Fatalf("menu publication retried rate-limited profile: %v", sender.profile)
	}
}

func TestLaunchButtonUpdatesBeforeCommandFailure(t *testing.T) {
	failure := errors.New("commands unavailable")
	sender := &menuSenderFake{commandErr: failure}
	const launch = "https://example.test/app/?v=2.0.0"
	if err := PublishMenus(context.Background(), sender, launch); !errors.Is(err, failure) {
		t.Fatalf("command error = %v", err)
	}
	if sender.globalURL != launch {
		t.Fatalf("launch button stayed stale: %q", sender.globalURL)
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

func TestStartupSharesMenuPublicationGate(t *testing.T) {
	sender := &menuSenderFake{}
	w := &Worker{Menus: sender, MiniApp: "https://example.test/app/", AppVersion: "test"}
	w.PublishMenuDefaults(t.Context())
	calls := len(sender.commandLanguages)
	if calls == 0 {
		t.Fatal("startup did not publish")
	}
	w.menusPub.Publish(t.Context(), sender, w.miniURL(""), nil)
	if len(sender.commandLanguages) != calls {
		t.Fatal("first command repeated startup publication")
	}
}
