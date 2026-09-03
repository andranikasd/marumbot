package telegramclient_test

import (
	"errors"
	"testing"

	"github.com/andranikasd/marumbot/internal/adapter/out/telegramclient"
)

func TestPrivateChatRequiredBeforeNetwork(t *testing.T) {
	client := telegramclient.New("unused").WithBase(":invalid")
	for _, id := range []int64{0, -1, -1001234567890} {
		calls := []func() error{
			func() error { return client.SendMessage(t.Context(), id, "private financial record", nil) },
			func() error { return client.SendChatAction(t.Context(), id, "typing") },
			func() error { return client.SetChatMenuButtonFor(t.Context(), id, "Open", "https://example.test") },
		}
		for _, call := range calls {
			if err := call(); !errors.Is(err, telegramclient.ErrPrivateChat) {
				t.Fatalf("non-private destination reached transport: %v", err)
			}
		}
	}
}
