package db

import (
	"database/sql"
	"strconv"
	"strings"

	"github.com/fatih/camelcase"
	"github.com/rah-0/nabu"

	"github.com/rah-0/margo/conf"
	"github.com/rah-0/margo/util"
)

func GetDbTables(c *sql.DB) ([]string, error) {
	var tables []string

	rows, err := c.Query(`
	SELECT table_name AS tableName
	FROM information_schema.tables
	WHERE table_schema = ?
	  AND table_type = 'BASE TABLE'
	ORDER BY table_name`,
		conf.Args.DBName,
	)
	if err != nil {
		return tables, nabu.FromError(err).Log()
	}

	for rows.Next() {
		var tableName string
		if err = rows.Scan(&tableName); err != nil {
			return tables, nabu.FromError(err).Log()
		}
		tables = append(tables, tableName)
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
	var tfs []conf.TableField
	rows, err := c.Query(`
		SELECT 
			COLUMN_NAME as columnName,
			DATA_TYPE as dataType,
			COLUMN_TYPE as columnType
		FROM 
			INFORMATION_SCHEMA.COLUMNS
		WHERE 
			table_name = '` + tableName + `'
				AND 
					table_schema = '` + conf.Args.DBName + `'
		ORDER BY 
			ORDINAL_POSITION
	`)
	if err != nil {
		return tfs, nabu.FromError(err).Log()
	}

	for rows.Next() {
		var columnName string
		var dataType string
		var columnType string

		if err = rows.Scan(&columnName, &dataType, &columnType); err != nil {
			return tfs, nabu.FromError(err).Log()
		}

		tfs = append(tfs, conf.TableField{
			Name:       columnName,
			DataType:   dataType,
			ColumnType: columnType,
			GOType:     IdentifyGOType(dataType, columnType),
		})
	}

	return tfs, nil
}

func IdentifyGOType(dataType, columnType string) string {
	dt := strings.ToLower(dataType)
	ct := strings.ToLower(columnType)
	isUnsigned := strings.Contains(ct, "unsigned")

	switch dt {
	case "tinyint", "smallint", "mediumint", "int", "integer":
		if isUnsigned {
			return "uint"
		}
		return "int"
	case "bigint":
		if isUnsigned {
			return "uint64"
		}
		return "int64"
	case "float", "double", "real":
		return "float64"
	case "decimal", "dec", "numeric", "fixed":
		return "decimal.Decimal"
	case "bit":
		m := conf.BitSizeRegex.FindStringSubmatch(ct)
		if len(m) == 2 {
			bitSize, _ := strconv.Atoi(m[1])
			switch {
			case bitSize == 1:
				return "bool"
			case bitSize <= 64:
				return "uint64"
			default:
				return "[]byte"
			}
		}
		return "uint64"
	case "bool", "boolean":
		return "bool"
	case "char":
		if ct == "char(36)" {
			return "uuid.UUID"
		}
		return "string"
	case "binary":
		return "[]byte"
	case "varchar", "text", "tinytext", "mediumtext", "longtext", "enum", "set":
		return "string"
	case "varbinary", "blob", "tinyblob", "mediumblob", "longblob":
		return "[]byte"
	case "date", "time", "year", "datetime", "timestamp":
		return "time.Time"
	case "uuid":
		return "uuid.UUID"
	}

	panic("unhandled DB type: " + dataType + " (" + columnType + ")")
}
