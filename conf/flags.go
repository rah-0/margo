package conf

import (
	"flag"
	"fmt"
	"os"

	_ "github.com/go-sql-driver/mysql"
)

func CheckFlags() {
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
		fmt.Fprintln(os.Stderr, "Missing required arguments:")
		for _, arg := range missing {
			fmt.Fprintln(os.Stderr, " ", arg)
		}
		flag.Usage()
		return
	}

	// Validate queriesPath is a directory if specified
	if *queriesPath != "" {
		info, err := os.Stat(*queriesPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: queriesPath '%s' does not exist or is not accessible: %v\n", *queriesPath, err)
			return
		}
		if !info.IsDir() {
			fmt.Fprintf(os.Stderr, "Error: queriesPath '%s' must be a directory, not a file\n", *queriesPath)
			return
		}
	}

	Args.DBUser = *dbUser
	Args.DBPassword = *dbPassword
	Args.DBName = *dbName
	Args.DBIp = *dbIp
	Args.DBPort = *dbPort
	Args.OutputPath = *outputPath
	Args.QueriesPath = *queriesPath // can be empty
}
