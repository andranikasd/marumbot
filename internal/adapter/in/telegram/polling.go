package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// UpdatePoller is the transport needed by the local inbound adapter.
type UpdatePoller interface {
	PollUpdates(context.Context, int64) ([]json.RawMessage, error)
}

// Poll runs until cancellation. The caller owns and joins its goroutine.
// No webhook is deleted automatically: using a hosted bot token locally must
// not silently take over that bot. Telegram rejects that conflict explicitly.
func (h *Webhook) Poll(ctx context.Context, source UpdatePoller) {
	var offset int64
	for ctx.Err() == nil {
		next, err := h.pollOnce(ctx, source, offset)
		offset = next
		if err == nil {
			continue
		}
		if ctx.Err() != nil {
			return
		}
		h.Log.WarnContext(ctx, "Telegram polling failed; retrying", "error", err)
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (h *Webhook) pollOnce(ctx context.Context, source UpdatePoller, offset int64) (int64, error) {
	updates, err := source.PollUpdates(ctx, offset)
	if err != nil {
		return offset, err
	}
	for _, raw := range updates {
		var update Update
		if err := json.Unmarshal(raw, &update); err != nil {
			return offset, fmt.Errorf("decode polled update: %w", err)
		}
		if update.UpdateID < offset {
			continue
		}
		if err := h.accept(ctx, update); err != nil {
			return offset, err
		}
		offset = update.UpdateID + 1
	}
	return offset, nil
}
