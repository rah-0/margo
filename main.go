package main

import (
	"github.com/rah-0/nabu"

	"github.com/rah-0/margo/conf"
	"github.com/rah-0/margo/db"
)

func main() {
	conf.CheckFlags()

	conn, err := db.Connect()
	if err != nil {
		nabu.FromError(err).Log()
		return
	}
	defer conn.Close()

}
