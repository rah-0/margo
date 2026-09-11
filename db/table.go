package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/fatih/camelcase"

	"github.com/rah-0/margo/conf"
	"github.com/rah-0/margo/migrate"
	"github.com/rah-0/margo/util"
)

func GetDbTables(c *sql.DB) ([]string, error) {
	return GetDbTablesContext(context.Background(), c)
}

// GetDbTablesContext lists application tables, excluding migration metadata.
func GetDbTablesContext(ctx context.Context, c *sql.DB) ([]string, error) {
	var tables []string

	rows, err := c.QueryContext(ctx, `
	SELECT table_name AS tableName
	FROM information_schema.tables
	WHERE table_schema = ?
	  AND table_type = 'BASE TABLE'
	  AND BINARY table_name <> ?
	ORDER BY table_name`,
		conf.Args.DBName,
		migrate.TableName,
	)
	if err != nil {
		return tables, fmt.Errorf("query database tables: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var tableName string
		if err = rows.Scan(&tableName); err != nil {
			return tables, fmt.Errorf("scan database table name: %w", err)
		}
		tables = append(tables, tableName)
	}
	if err := rows.Err(); err != nil {
		return tables, fmt.Errorf("read database tables: %w", err)
	}

	return tables, nil
}

var separators = []rune{'_', '-', '.'}

func NormalizeString(input string) string {
	// Replace all separators with " "
	for _, sep := range separators {
		input = strings.ReplaceAll(input, string(sep), " ")
	}

	// snake_case path
	if strings.Contains(input, " ") {
		parts := strings.Split(input, " ")
		for i, p := range parts {
			if p != "" {
				parts[i] = util.Capitalize(p)
			}
		}
		return strings.Join(parts, "")
	}

	// fallback to camel case
	parts := camelcase.Split(input)
	for i, p := range parts {
		parts[i] = util.Capitalize(p)
	}
	return strings.Join(parts, "")
}

func GetDbTableFields(c *sql.DB, tableName string) ([]conf.TableField, error) {
	return GetDbTableFieldsContext(context.Background(), c, tableName)
}

// GetDbTableFieldsContext reads the columns of a table with cancellation support.
func GetDbTableFieldsContext(ctx context.Context, c *sql.DB, tableName string) ([]conf.TableField, error) {
	var tfs []conf.TableField
	rows, err := c.QueryContext(ctx, `
		SELECT 
			COLUMN_NAME as columnName,
			DATA_TYPE as dataType,
			COLUMN_TYPE as columnType
		FROM 
			INFORMATION_SCHEMA.COLUMNS
		WHERE 
			table_name = ?
				AND 
					table_schema = ?
		ORDER BY 
			ORDINAL_POSITION
	`, tableName, conf.Args.DBName)
	if err != nil {
		return tfs, fmt.Errorf("query fields for table %q: %w", tableName, err)
	}
	defer rows.Close()

	for rows.Next() {
		var columnName string
		var dataType string
		var columnType string

		if err = rows.Scan(&columnName, &dataType, &columnType); err != nil {
			return tfs, fmt.Errorf("scan fields for table %q: %w", tableName, err)
		}

		tfs = append(tfs, conf.TableField{
			Name:       columnName,
			DataType:   dataType,
			ColumnType: columnType,
		})
	}
	if err := rows.Err(); err != nil {
		return tfs, fmt.Errorf("read fields for table %q: %w", tableName, err)
	}

	return tfs, nil
}
