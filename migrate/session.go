package migrate

import (
	"context"
	"database/sql"
	"fmt"
)

func validateSession(ctx context.Context, conn *sql.Conn, expectedDatabase string) (string, error) {
	var database sql.NullString
	var autocommit, inTransaction bool
	if err := conn.QueryRowContext(ctx, "SELECT DATABASE(), @@autocommit, @@in_transaction").Scan(
		&database, &autocommit, &inTransaction,
	); err != nil {
		return "", fmt.Errorf("migrate: read session state: %w", err)
	}
	if !database.Valid || database.String == "" {
		return "", fmt.Errorf("%w: %w", ErrInvalidSessionState, ErrDatabaseNotSelected)
	}
	if expectedDatabase != "" && database.String != expectedDatabase {
		return "", fmt.Errorf("%w: expected database %q, got %q", ErrInvalidSessionState, expectedDatabase, database.String)
	}
	if !autocommit || inTransaction {
		return "", fmt.Errorf("%w: require autocommit enabled and no open transaction (autocommit=%t, in_transaction=%t)",
			ErrInvalidSessionState, autocommit, inTransaction)
	}
	return database.String, nil
}
