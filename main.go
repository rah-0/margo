package main

import (
	"github.com/rah-0/nabu"

	"github.com/rah-0/margo/conf"
	"github.com/rah-0/margo/db"
	"github.com/rah-0/margo/template"
)

func main() {
	conf.CheckFlags()

	conn, err := db.Connect()
	if err != nil {
		nabu.FromError(err).Log()
		return
	}
	defer conn.Close()

	if err = template.PathCreateOutputDir(); err != nil {
		nabu.FromError(err).WithLevelFatal().Log()
		return
	}

	if err = template.PathCreateDBDir(); err != nil {
		nabu.FromError(err).WithLevelFatal().Log()
		return
	}

	tableNames, err := db.GetDbTables(conn)
	if err != nil {
		nabu.FromError(err).WithLevelFatal().Log()
		return
	}

	if err = template.PathCreateTableDirs(tableNames); err != nil {
		nabu.FromError(err).WithLevelFatal().Log()
		return
	}

	for _, tn := range tableNames {
		tfs, err := db.GetDbTableFields(conn, tn)
		if err != nil {
			nabu.FromError(err).WithLevelFatal().Log()
			return
		}
		if err := template.CreateGoFileEntity(tn, tfs); err != nil {
			nabu.FromError(err).WithLevelFatal().Log()
			return
		}
	}

	if err := template.CreateGoFileQueries(); err != nil {
		nabu.FromError(err).WithLevelFatal().Log()
		return
	}
}
