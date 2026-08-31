package app

import (
	"context"
	"fmt"
	"sync"

	"github.com/andranikasd/marumbot/internal/i18n"
)

// MenuPublisher is the Telegram surface used to describe the bot to itself.
type MenuPublisher interface {
	SetMyCommands(ctx context.Context, lang string, cmds []BotCommand) error
	SetChatMenuButton(ctx context.Context, text, url string) error
}

// BotCommand mirrors the client's type so this package does not import an
// adapter.
type BotCommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}

// commandKeys pairs each command with the catalogue key describing it. The
// order is the order Telegram shows them, so it runs from what a new user needs
// first to what they need rarely.
var commandKeys = []struct{ cmd, key string }{
	{"add", "menu.add"},
	{"advice", "menu.advice"},
	{"loans", "menu.loans"},
	{"budget", "menu.budget"},
	{"language", "menu.language"},
	{"help", "menu.help"},
}

// PublishOnce runs PublishMenus at most once per process.
//
// Startup is the obvious place to publish, and it is not sufficient: a
// container only starts when a request arrives and survives across Worker
// deploys until it sleeps, so a deploy can leave a running process that never
// re-ran its startup path. That is exactly what happened -- the deploy
// succeeded, the smoke test passed, and the chat still offered no commands.
//
// Calling this from the tick as well makes publication depend on the bot being
// alive rather than on when it happened to start.
func PublishOnce(ctx context.Context, once *sync.Once, p MenuPublisher, miniAppURL string) {
	once.Do(func() {
		if err := PublishMenus(ctx, p, miniAppURL); err != nil {
			// Not fatal, and not retried: a bot with no command suggestions is
			// worse to use but still works, and hammering a rate-limited
			// endpoint on every tick would be worse than the missing menu.
			_ = err
		}
	})
}

// PublishMenus tells Telegram what this bot can do, in every language it
// speaks, and points the chat's menu button at the loan form.
//
// It runs at startup rather than per conversation because the lists are
// per-bot, not per-user: publishing them on every message would be thousands of
// identical calls against a rate limit that exists for a reason.
//
// A failure is logged and not fatal. A bot with no command suggestions is worse
// to use but still works; a bot that refuses to start because Telegram was busy
// is worse than that.
func PublishMenus(ctx context.Context, p MenuPublisher, miniAppURL string) error {
	for _, l := range i18n.Supported() {
		cmds := make([]BotCommand, 0, len(commandKeys))
		for _, c := range commandKeys {
			cmds = append(cmds, BotCommand{Command: c.cmd, Description: i18n.T(l, c.key)})
		}
		if err := p.SetMyCommands(ctx, string(l), cmds); err != nil {
			return fmt.Errorf("publishing %s commands: %w", l, err)
		}
		// The default scope catches every language Marum does not speak. Armenian
		// is the right fallback here for the same reason it is the default
		// locale.
		if l == i18n.Default {
			if err := p.SetMyCommands(ctx, "", cmds); err != nil {
				return fmt.Errorf("publishing default commands: %w", err)
			}
		}
	}

	if miniAppURL != "" {
		// Armenian, because the button is global and cannot be per-user.
		if err := p.SetChatMenuButton(ctx, i18n.T(i18n.Default, "btn.add"), miniAppURL); err != nil {
			return fmt.Errorf("setting the menu button: %w", err)
		}
	}
	return nil
}
