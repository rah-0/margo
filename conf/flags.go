package conf

import (
	"flag"
	"fmt"
	"os"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

func CheckFlags() error {
	var args Arguments
	flag.StringVar(&args.DBUser, "dbUser", "", "Required")
	flag.StringVar(&args.DBPassword, "dbPassword", "", "Required")
	flag.StringVar(&args.DBName, "dbName", "", "Required")
	flag.StringVar(&args.DBIp, "dbIp", "", "Required")
	flag.StringVar(&args.DBPort, "dbPort", "3306", "Required")
	flag.StringVar(&args.OutputPath, "outputPath", "", "Optional: path where .go files will be created; omit to skip code generation.")
	flag.StringVar(&args.QueriesPath, "queriesPath", "", "Optional: path to directory containing .sql query files.")
	flag.StringVar(&args.MigrationsPath, "migrationsPath", "", "Optional: path to numbered SQL migrations applied before code generation.")
	flag.Parse()

	if args.OutputPath == "" && args.QueriesPath == "" && args.MigrationsPath == "" {
		Args = args
		return nil
	}
	if args.QueriesPath != "" && args.OutputPath == "" {
		return fmt.Errorf("%w: -outputPath is required when -queriesPath is set", ErrMissingArgs)
	}

	var missing []string
	if args.DBUser == "" {
		missing = append(missing, "-dbUser")
	}
	if args.DBPassword == "" {
		missing = append(missing, "-dbPassword")
	}
	if args.DBName == "" {
		missing = append(missing, "-dbName")
	}
	if args.DBIp == "" {
		missing = append(missing, "-dbIp")
	}
	if args.DBPort == "" {
		missing = append(missing, "-dbPort")
	}
	if len(missing) > 0 {
		flag.Usage()
		return fmt.Errorf("%w: %s", ErrMissingArgs, strings.Join(missing, ", "))
	}

	if args.OutputPath != "" {
		if err := validateOutputPath(args.OutputPath); err != nil {
			return err
		}
	}
	if args.QueriesPath != "" {
		info, err := os.Stat(args.QueriesPath)
		if err != nil {
			return fmt.Errorf("%w: %q: %w", ErrQueriesPathInvalid, args.QueriesPath, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("%w: %q", ErrQueriesPathNotDir, args.QueriesPath)
		}
	}
	if args.MigrationsPath != "" {
		info, err := os.Stat(args.MigrationsPath)
		if err != nil {
			return fmt.Errorf("%w: %q: %w", ErrMigrationsPathInvalid, args.MigrationsPath, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("%w: %q", ErrMigrationsPathNotDir, args.MigrationsPath)
		}
	}

	Args = args
	return nil
}
