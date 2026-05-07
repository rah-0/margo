package main

import (
	"github.com/rah-0/nabu"

	"github.com/rah-0/margo/conf"
	"github.com/rah-0/margo/db"
	"github.com/rah-0/margo/template"
)

func main() {
	nabu.SetFormatter(&nabu.PlainFormatter{
		Colored:    true,
		EnableDate: true,
		EnableArgs: true,
	})

	if err := conf.CheckFlags(); err != nil {
		nabu.FromError(err).WithLevelFatal().Log()
		return
	}

	conn, err := db.Connect()
	if err != nil {
		nabu.FromError(err).Log()
		return
	}
	defer func() {
		if err := conn.Close(); err != nil {
			nabu.FromError(err).Log()
		}
	}()

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

	nqs, err := template.CreateGoFileQueries(tableNames)
	if err != nil {
		nabu.FromError(err).WithLevelFatal().Log()
		return
	}

	for _, tn := range tableNames {
		tfs, err := db.GetDbTableFields(conn, tn)
		if err != nil {
			nabu.FromError(err).WithLevelFatal().Log()
			return
		}

		tnqs := []conf.NamedQuery{}
		for _, nq := range nqs {
			if nq.MapAs == tn {
				tnqs = append(tnqs, nq)
			}
		}

		if err := template.CreateGoFileEntity(tn, tfs, tnqs); err != nil {
			nabu.FromError(err).WithLevelFatal().Log()
			return
		}
	}
}
