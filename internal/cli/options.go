// Package cli parses command-line options for the MarGO executable.
package cli

import (
	"flag"
	"io"

	"github.com/rah-0/margo/runner"
	"github.com/rah-0/margo/structs"
)

// ParseOptions parses one invocation using an independent flag set. Flag errors
// and usage are written to output; connection and path validation belong to runner.Run.
func ParseOptions(args []string, output io.Writer) (runner.Options, error) {
	connection := new(structs.ConnectionOptions)
	opts := runner.Options{Connection: connection}
	flags := flag.NewFlagSet("margo", flag.ContinueOnError)
	flags.SetOutput(output)
	flags.StringVar(&connection.User, "dbUser", "", "Required")
	flags.StringVar(&connection.Password, "dbPassword", "", "Required")
	flags.StringVar(&connection.Database, "dbName", "", "Required")
	flags.StringVar(&connection.Host, "dbIp", "", "Required")
	flags.StringVar(&connection.Port, "dbPort", "3306", "Required")
	flags.StringVar(&opts.OutputPath, "outputPath", "", "Optional: path where .go files will be created; omit to skip code generation.")
	flags.StringVar(&opts.QueriesPath, "queriesPath", "", "Optional: path to directory containing .sql query files.")
	flags.StringVar(&opts.MigrationsPath, "migrationsPath", "", "Optional: path to numbered SQL migrations applied before code generation.")
	err := flags.Parse(args)
	return opts, err
}
