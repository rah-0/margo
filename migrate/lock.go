package migrate

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"time"
)

func lockName(database string) string {
	// A fixed-size name also supports databases near the identifier length limit.
	return fmt.Sprintf("%x", sha256.Sum256([]byte(TableName+":"+database)))
}

func acquireLock(ctx context.Context, conn *sql.Conn, name string) error {
	var acquired sql.NullInt64
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, 30)", name).Scan(&acquired); err != nil {
		return fmt.Errorf("%w: %w", ErrLockFailed, err)
	}
	if !acquired.Valid {
		return ErrLockFailed
	}
	if acquired.Int64 == 0 {
		return ErrLockTimeout
	}
	if acquired.Int64 != 1 {
		return fmt.Errorf("%w: GET_LOCK returned %d", ErrLockFailed, acquired.Int64)
	}
	return nil
}

func releaseLock(conn *sql.Conn, name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var released sql.NullInt64
	err := conn.QueryRowContext(ctx, "SELECT RELEASE_LOCK(?)", name).Scan(&released)
	if errors.Is(err, sql.ErrConnDone) {
		// A prior driver failure already discarded this dedicated connection.
		return nil
	}
	if err == nil && released.Valid && released.Int64 == 1 {
		return nil
	}
	if err == nil {
		err = fmt.Errorf("RELEASE_LOCK did not confirm release")
	}
	return fmt.Errorf("migrate: release advisory lock: %w", err)
}

func discardConnection(conn *sql.Conn) error {
	// Returning driver.ErrBadConn from Raw makes database/sql close the physical
	// connection. Conn.Close would return the session to the caller's pool.
	err := conn.Raw(func(any) error { return driver.ErrBadConn })
	if errors.Is(err, driver.ErrBadConn) || errors.Is(err, sql.ErrConnDone) {
		return nil
	}
	return err
}
