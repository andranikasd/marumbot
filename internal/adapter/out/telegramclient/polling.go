package telegramclient

import (
	"context"
	"encoding/json"
)

// PollUpdates reads a small long-poll batch. A short server wait fits within the
// shared client's ten-second transport deadline. Advancing offset acknowledges
// older updates, so the inbound adapter advances it only after durable enqueue.
func (c *Client) PollUpdates(ctx context.Context, offset int64) ([]json.RawMessage, error) {
	var updates []json.RawMessage
	err := c.callResult(ctx, "getUpdates", map[string]any{
		"offset": offset, "timeout": 5, "limit": 1,
		"allowed_updates": []string{"message", "callback_query"},
	}, &updates)
	return updates, err
}
