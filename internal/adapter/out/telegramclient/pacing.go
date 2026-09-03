package telegramclient

import (
	"context"
	"sync"
	"time"

	"github.com/andranikasd/marumbot/internal/app"
)

// Conservative admission spacing: all API methods share 25 calls/second;
// sendMessage additionally shares one message/second per private chat.
// https://core.telegram.org/bots/faq#my-bot-is-hitting-limits-how-do-i-avoid-this
const callSpacing = 40 * time.Millisecond

type pacing struct {
	mu      sync.Mutex
	clock   app.Clock
	next    time.Time
	blocked time.Time
	chats   map[int64]time.Time
}

func (p *pacing) wait(ctx context.Context, chat int64) error {
	for {
		p.mu.Lock()
		if err := ctx.Err(); err != nil {
			p.mu.Unlock()
			return err
		}
		now := p.clock.Now()
		due := p.next
		if p.blocked.After(due) {
			due = p.blocked
		}
		if p.chats[chat].After(due) {
			due = p.chats[chat]
		}
		if !due.After(now) {
			// Retain only active chat windows, not a growing account registry.
			for id, until := range p.chats {
				if !until.After(now) {
					delete(p.chats, id)
				}
			}
			p.next = now.Add(callSpacing)
			if chat != 0 {
				p.chats[chat] = now.Add(time.Second)
			}
			p.mu.Unlock()
			return nil
		}
		p.mu.Unlock()
		// No reservation while waiting: a cancelled call consumes no slot,
		// and a busy chat cannot hold up a different chat or a callback answer.
		timer := time.NewTimer(due.Sub(now))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		// Recheck after waking: another call may have set a longer cooldown.
	}
}

func (p *pacing) cooldown(wait time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if until := p.clock.Now().Add(wait); until.After(p.blocked) {
		p.blocked = until
	}
}
