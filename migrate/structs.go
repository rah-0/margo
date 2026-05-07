package migrate

import (
	"database/sql"
	"time"
)

type Options struct {
	DB              *sql.DB
	Path            string
	LockName        string
	LockTimeout     time.Duration
	TableName       string
	Strict          bool
	UseTransactions bool
}

type Migration struct {
	Version  uint64
	Name     string
	UpPath   string
	DownPath string
	UpHash   string
	DownHash string
}

type MigrationStatus struct {
	Version     uint64
	Name        string
	UpHash      string
	DownHash    sql.NullString
	State       State
	StartedAt   time.Time
	FinishedAt  sql.NullTime
	ExecutionMs sql.NullInt64
	ErrorText   sql.NullString
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
