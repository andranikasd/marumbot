package app

import "context"

// LegacyAdminSecret is an existing credential awaiting encryption, never telemetry.
type LegacyAdminSecret struct {
	ID     string
	Secret string
}
type AdminSecretMigrationStore interface {
	LegacyAdminSecrets(context.Context) ([]LegacyAdminSecret, error)
	ProtectAdminSecret(context.Context, LegacyAdminSecret) error
}

// ProtectAdminSecrets is restartable and compare-and-set guarded against enrollments.
func ProtectAdminSecrets(ctx context.Context, s AdminSecretMigrationStore) error {
	for {
		rows, err := s.LegacyAdminSecrets(ctx)
		if err != nil {
			return err
		}
		for _, row := range rows {
			if err := s.ProtectAdminSecret(ctx, row); err != nil {
				return err
			}
		}
		if len(rows) < 100 {
			return nil
		}
	}
}
