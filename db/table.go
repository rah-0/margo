package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/rah-0/margo/migrate"
	"github.com/rah-0/margo/structs"
	"github.com/rah-0/margo/util"
)

// GetDbTablesContext lists application tables, excluding migration metadata.
func GetDbTablesContext(ctx context.Context, c *sql.DB, schema string) ([]string, error) {
	var tables []string

	rows, err := c.QueryContext(ctx, `
	SELECT table_name AS tableName
	FROM information_schema.tables
	WHERE table_schema = ?
	  AND table_type = 'BASE TABLE'
	  AND BINARY table_name <> ?
	ORDER BY table_name`,
		schema,
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

// NormalizeString normalizes database identifiers for generated names.
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

	// Capitalize invalid UTF-8 as one word.
	if !utf8.ValidString(input) {
		return util.Capitalize(input)
	}

	runes := []rune(input)
	previousClass := runeClassOther
	for i, r := range runes {
		class := runeClassOther
		switch {
		case unicode.IsLower(r):
			class = runeClassLower
		case unicode.IsUpper(r):
			class = runeClassUpper
		case unicode.IsDigit(r):
			class = runeClassDigit
		}

		startsWord := i == 0 || class != previousClass
		// Keep a capital with its lowercase suffix: Server is one word.
		if previousClass == runeClassUpper && class == runeClassLower {
			startsWord = false
		}
		// The final capital in an acronym starts the next word: HTTPServer.
		if class == runeClassUpper && i+1 < len(runes) && unicode.IsLower(runes[i+1]) {
			startsWord = true
		}
		if startsWord {
			runes[i] = unicode.ToUpper(r)
		} else {
			runes[i] = unicode.ToLower(r)
		}
		previousClass = class
	}
	return string(runes)
}

// GetDbTableFieldsContext reads the columns of a table with cancellation support.
func GetDbTableFieldsContext(ctx context.Context, c *sql.DB, schema, tableName string) ([]structs.TableField, error) {
	var tfs []structs.TableField
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
	`, tableName, schema)
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

		tfs = append(tfs, structs.TableField{
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
