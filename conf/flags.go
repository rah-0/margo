package conf

import (
	"flag"
	"fmt"
	"os"
	"strings"

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
		flag.Usage()
		return fmt.Errorf("%w: %s", ErrMissingArgs, strings.Join(missing, ", "))
	}

	// Validate queriesPath is a directory if specified
	if *queriesPath != "" {
		info, err := os.Stat(*queriesPath)
		if err != nil {
			return fmt.Errorf("%w: %q: %w", ErrQueriesPathInvalid, *queriesPath, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("%w: %q", ErrQueriesPathNotDir, *queriesPath)
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
