package db

import (
	"database/sql"
	"fmt"
	"runtime"
	"time"

	"github.com/rah-0/margo/conf"
)

func Connect() (*sql.DB, error) {
	conn, err := sql.Open("mysql", conf.Args.DBUser+":"+conf.Args.DBPassword+"@tcp("+conf.Args.DBIp+":"+conf.Args.DBPort+")/"+conf.Args.DBName)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	conn.SetMaxIdleConns(runtime.NumCPU())
	conn.SetConnMaxLifetime(time.Minute * 5)
	conn.SetConnMaxIdleTime(time.Minute * 1)

	if err = conn.Ping(); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return conn, nil
}
