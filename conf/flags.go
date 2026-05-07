package conf

import (
	"flag"
	"os"

	"github.com/rah-0/nabu"

	_ "github.com/go-sql-driver/mysql"
)

func CheckFlags() error {
	dbUser := flag.String("dbUser", "", "Required")
	dbPassword := flag.String("dbPassword", "", "Required")
	dbName := flag.String("dbName", "", "Required")
	dbIp := flag.String("dbIp", "", "Required")
	dbPort := flag.String("dbPort", "3306", "Required")
	outputPath := flag.String("outputPath", "", "Required: path where .go files will be created.")
	queriesPath := flag.String("queriesPath", "", "Optional: path to directory containing .sql query files.")
	flag.Parse()

	var missing []string

	if *dbUser == "" {
		missing = append(missing, "-dbUser")
	}
	if *dbPassword == "" {
		missing = append(missing, "-dbPassword")
	}
	if *dbName == "" {
		missing = append(missing, "-dbName")
	}
	if *dbIp == "" {
		missing = append(missing, "-dbIp")
	}
	if *dbPort == "" {
		missing = append(missing, "-dbPort")
	}
	if *outputPath == "" {
		missing = append(missing, "-outputPath")
	}

	if len(missing) > 0 {
		args := make([]any, len(missing))
		for i, m := range missing {
			args[i] = m
		}
		flag.Usage()
		return nabu.FromError(ErrMissingArgs).WithArgs(args...).Log()
	}

	// Validate queriesPath is a directory if specified
	if *queriesPath != "" {
		info, err := os.Stat(*queriesPath)
		if err != nil {
			return nabu.FromError(ErrQueriesPathInvalid).WithArgs(*queriesPath, err).Log()
		}
		if !info.IsDir() {
			return nabu.FromError(ErrQueriesPathNotDir).WithArgs(*queriesPath).Log()
		}
	}

	Args.DBUser = *dbUser
	Args.DBPassword = *dbPassword
	Args.DBName = *dbName
	Args.DBIp = *dbIp
	Args.DBPort = *dbPort
	Args.OutputPath = *outputPath
	Args.QueriesPath = *queriesPath // can be empty
	return nil
}
