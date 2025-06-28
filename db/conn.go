package db

import (
	"database/sql"
	"runtime"
	"time"

	"github.com/rah-0/nabu"

	"github.com/rah-0/margo/conf"
)

func Connect() (*sql.DB, error) {
	conn, err := sql.Open("mysql", conf.Args.DBUser+":"+conf.Args.DBPassword+"@tcp("+conf.Args.DBIp+":"+conf.Args.DBPort+")/"+conf.Args.DBName)
	if err != nil {
		return nil, nabu.FromError(err).Log()
	}

	conn.SetMaxIdleConns(runtime.NumCPU())
	conn.SetConnMaxLifetime(time.Minute * 5)
	conn.SetConnMaxIdleTime(time.Minute * 1)

	if err = conn.Ping(); err != nil {
		return nil, nabu.FromError(err).Log()
	}

	return conn, nil
}
