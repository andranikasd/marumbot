package app

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"

	"github.com/andranikasd/marumbot/internal/i18n"
)

// MenuPublisher is the Telegram surface used to describe the bot to itself.
type MenuPublisher interface {
	SetMyCommands(ctx context.Context, lang string, cmds []BotCommand) error
	SetChatMenuButton(ctx context.Context, text, url string) error
}

// MenuUser is the minimum account data needed to refresh a per-chat button.
type MenuUser struct {
	ID, Locale string
}

// MenuUserLister pages through accounts with Telegram menu buttons. It is a
// separate capability from the admin user list and does not expose identities.
type MenuUserLister interface {
	MenuUsers(ctx context.Context, after string, limit int32) ([]MenuUser, error)
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

// MenuPublication runs PublishMenus at most once per process.
//
// Startup is the obvious place to publish, and it is not sufficient: a
// container only starts when a request arrives and survives across Worker
// deploys until it sleeps, so a deploy can leave a running process that never
// re-ran its startup path. That is exactly what happened -- the deploy
// succeeded, the smoke test passed, and the chat still offered no commands.
//
// Calling this from the tick as well makes publication depend on the bot being
// alive rather than on when it happened to start.
//
// Done is only recorded on success, so a failed publish is retried on a later
// call instead of being silently spent -- one attempt at a time, and the
// failure itself is logged, as the doc comments on PublishMenus promise.
type MenuPublication struct {
	done atomic.Bool
	busy atomic.Bool
}

// Publish runs PublishMenus once per process, retrying on later calls until it
// has succeeded.
func (m *MenuPublication) Publish(ctx context.Context, p MenuPublisher, miniAppURL string, log *slog.Logger) {
	if m.done.Load() || !m.busy.CompareAndSwap(false, true) {
		return
	}
	defer m.busy.Store(false)
	if err := PublishMenus(ctx, p, miniAppURL); err != nil {
		if log != nil {
			log.WarnContext(ctx, "publishing the command menus failed", "error", err)
		}
		return
	}
	m.done.Store(true)
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
		if err := p.SetChatMenuButton(ctx, i18n.DashboardButton(i18n.Default), miniAppURL); err != nil {
			return fmt.Errorf("setting the menu button: %w", err)
		}
	}
	return nil
}

// RefreshMenuButtons replaces every per-chat override left by an older
// deployment. Telegram does not let a new global default override an existing
// chat-specific button, so rollout propagation must update those explicitly.
func (w *Worker) RefreshMenuButtons(ctx context.Context, users MenuUserLister) (int, error) {
	if users == nil || w.Chats == nil || w.Send == nil || w.MiniApp == "" {
		return 0, nil
	}
	const pageSize int32 = 100
	after, refreshed, failed := "", 0, 0
	for {
		page, err := users.MenuUsers(ctx, after, pageSize)
		if err != nil {
			return refreshed, fmt.Errorf("listing menu accounts: %w", err)
		}
		for _, user := range page {
			chatID, err := w.Chats.ChatID(ctx, user.ID)
			if err == nil {
				locale := i18n.Locale(user.Locale)
				err = w.Send.SetChatMenuButtonFor(ctx, chatID, i18n.DashboardButton(locale), w.miniURL(""))
			}
			if err != nil {
				failed++
				continue
			}
			refreshed++
		}
		if len(page) < int(pageSize) {
			break
		}
		after = page[len(page)-1].ID
	}
	if failed > 0 {
		return refreshed, fmt.Errorf("refreshing menu buttons failed for %d accounts", failed)
	}
	return refreshed, nil
}
