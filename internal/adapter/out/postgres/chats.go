package postgres

import (
	"context"
	"fmt"

	"github.com/andranikasd/marumbot/internal/identity"
)

// ChatLookup resolves an account to the Telegram chat to reply to.
//
// It sits between the store and the worker because it needs both the row and
// the key, and neither of those belongs in the other's package: the store must
// not hold the key, and the worker must not hold a connection.
type ChatLookup struct {
	Store  *Store
	Cipher *identity.Cipher
}

// ChatID decrypts the chat identifier for an account.
func (c ChatLookup) ChatID(ctx context.Context, userID string) (int64, error) {
	box, ver, err := c.Store.ChatCipher(ctx, userID)
	if err != nil {
		return 0, err
	}
	if ver != identity.KeyVersion {
		// A row sealed under a retired key needs re-sealing, not guessing.
		return 0, fmt.Errorf("identity: chat sealed under key version %d, this build has %d",
			ver, identity.KeyVersion)
	}
	return c.Cipher.Open(box)
}
