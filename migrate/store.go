package migrate

import (
	"context"
	"database/sql"
	"fmt"
)

func initializeVersion(ctx context.Context, conn *sql.Conn) (uint64, error) {
	if _, err := conn.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS `"+TableName+"` ("+
		"`id` TINYINT UNSIGNED NOT NULL PRIMARY KEY CHECK (`id` = 1), "+
		"`version` BIGINT UNSIGNED NOT NULL) ENGINE=InnoDB"); err != nil {
		return 0, fmt.Errorf("migrate: create version table: %w", err)
	}
	if _, err := conn.ExecContext(ctx, "INSERT INTO `"+TableName+"` (`id`, `version`) "+
		"SELECT 1, 0 WHERE NOT EXISTS (SELECT 1 FROM `"+TableName+"` WHERE `id` = 1)"); err != nil {
		return 0, fmt.Errorf("migrate: initialize version: %w", err)
	}
	var version uint64
	if err := conn.QueryRowContext(ctx, "SELECT `version` FROM `"+TableName+"` WHERE `id` = 1").Scan(&version); err != nil {
		return 0, fmt.Errorf("migrate: read version: %w", err)
	}
	return version, nil
}

func saveVersion(ctx context.Context, conn *sql.Conn, previous, version uint64) error {
	result, err := conn.ExecContext(ctx, "UPDATE `"+TableName+"` SET `version` = ? WHERE `id` = 1 AND `version` = ?", version, previous)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("expected one version row to update, got %d", count)
	}
	return nil
}
