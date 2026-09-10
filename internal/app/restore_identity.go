package app

import "context"

// RestoreAdminVerifier checks every stored admin credential, including disabled
// accounts, without returning decrypted secrets to the recovery orchestrator.
type RestoreAdminVerifier interface {
	VerifyAdminSecrets(context.Context) error
}

// VerifyRestoredData checks the configured identity key and original plan replay.
// It performs no network delivery and retains no decrypted identifiers.
func VerifyRestoredData(ctx context.Context, users MenuUserLister, chats ChatResolver, history PlanHistoryStore, admins RestoreAdminVerifier) error {
	if admins == nil {
		return ErrAdminSecurityUnavailable
	}
	if err := admins.VerifyAdminSecrets(ctx); err != nil {
		return err
	}
	after := ""
	for {
		rows, err := users.MenuUsers(ctx, after, 100)
		if err != nil {
			return err
		}
		for _, row := range rows {
			if _, err := chats.ChatID(ctx, row.ID); err != nil {
				return err
			}
			versions, _, err := history.PlanHistory(ctx, row.ID)
			if err != nil {
				return err
			}
			for _, version := range versions {
				if _, err := ReplayManifest(version.Manifest); err != nil {
					return err
				}
			}
			after = row.ID
		}
		if len(rows) < 100 {
			return nil
		}
	}
}
