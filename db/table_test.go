package db

import (
	"testing"
)

func TestGetDbTables(t *testing.T) {
	tables, err := GetDbTables(conn)
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) == 0 {
		t.Fatal("Expected some tables")
	}
}

func TestNormalizeString(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		// basic expected formats
		{"table_name", "TableName"},
		{"Table_Name", "TableName"},
		{"tableName", "TableName"},
		{"TableName", "TableName"},
		{"TABLENAME", "Tablename"},
		{"entity_user_data", "EntityUserData"},
		{"userID", "UserId"},  // no acronym preservation
		{"user_id", "UserId"}, // normalized snake
		{"user1_data2", "User1Data2"},
		{"", ""},

		// edge casing
		{"_table_name", "TableName"},
		{"table_name_", "TableName"},
		{"__table__name__", "TableName"},
		{"table__name", "TableName"},
		{"user___id", "UserId"},

		// acronyms/numbers
		{"ip_address_v4", "IpAddressV4"},
		{"cpu_temp_stats", "CpuTempStats"},
		{"user2fa_status", "User2faStatus"},
		{"v2_api_users", "V2ApiUsers"},

		// camel case with acronyms
		{"userIDStats", "UserIdStats"},
		{"HTTPConnection", "HttpConnection"},
		{"OAuthToken", "OAuthToken"},

		// mixed case & garbage
		{"123abc", "123Abc"},
		{"abc123def", "Abc123Def"},
		{"___", ""},
		{"__", ""},
		{"_", ""},
		{"tableName!", "TableName!"},

		// unicode
		{"tést_tab", "TéstTab"},
		{"über_cool", "ÜberCool"},
		{"данные_пользователя", "ДанныеПользователя"},
	}

	for _, tt := range tests {
		result := NormalizeString(tt.input)
		if result != tt.expected {
			t.Errorf("NormalizeString(%q) = %q; want %q", tt.input, result, tt.expected)
		}
	}
}

func TestGetDbTableFields(t *testing.T) {
	tables, err := GetDbTables(conn)
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) == 0 {
		t.Fatal("Expected some tables")
	}

	for _, table := range tables {
		_, err := GetDbTableFields(conn, table)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestIdentifyGOType(t *testing.T) {
	tests := []struct {
		dataType   string
		columnType string
		expected   string
	}{
		// signed integer types
		{"tinyint", "tinyint(3)", "int"},
		{"smallint", "smallint(5)", "int"},
		{"mediumint", "mediumint(8)", "int"},
		{"int", "int(11)", "int"},
		{"integer", "integer(11)", "int"},

		// unsigned integer types
		{"tinyint", "tinyint(3) unsigned", "uint"},
		{"int", "int(11) unsigned", "uint"},
		{"bigint", "bigint(20) unsigned", "uint64"},
		{"bigint", "bigint(20)", "int64"},

		// float
		{"float", "float", "float64"},
		{"double", "double", "float64"},
		{"real", "real", "float64"},

		// decimal types
		{"decimal", "decimal(10,2)", "decimal.Decimal"},
		{"dec", "dec(10,2)", "decimal.Decimal"},
		{"numeric", "numeric(15,5)", "decimal.Decimal"},
		{"fixed", "fixed(7,3)", "decimal.Decimal"},

		// bit handling
		{"bit", "bit(1)", "bool"},
		{"bit", "bit(8)", "uint64"},
		{"bit", "bit(64)", "uint64"},
		{"bit", "bit(65)", "[]byte"},
		{"bit", "bit(128)", "[]byte"},
		{"bit", "bit", "uint64"}, // fallback

		// boolean aliases
		{"bool", "bool", "bool"},
		{"boolean", "boolean", "bool"},

		// string types
		{"char", "char(10)", "string"},
		{"varchar", "varchar(255)", "string"},
		{"text", "text", "string"},
		{"tinytext", "tinytext", "string"},
		{"mediumtext", "mediumtext", "string"},
		{"longtext", "longtext", "string"},
		{"enum", "enum('a','b')", "string"},
		{"set", "set('a','b')", "string"},

		// binary/blob
		{"binary", "binary(8)", "[]byte"},
		{"varbinary", "varbinary(255)", "[]byte"},
		{"blob", "blob", "[]byte"},
		{"tinyblob", "tinyblob", "[]byte"},
		{"mediumblob", "mediumblob", "[]byte"},
		{"longblob", "longblob", "[]byte"},

		// date/time
		{"date", "date", "time.Time"},
		{"time", "time", "time.Time"},
		{"year", "year", "time.Time"},
		{"datetime", "datetime(6)", "time.Time"},
		{"timestamp", "timestamp", "time.Time"},

		// uuid
		{"uuid", "uuid", "uuid.UUID"},
	}

	for _, tt := range tests {
		t.Run(tt.dataType+"_"+tt.columnType, func(t *testing.T) {
			got := IdentifyGOType(tt.dataType, tt.columnType)
			if got != tt.expected {
				t.Errorf("IdentifyGOType(%q, %q) = %q; want %q", tt.dataType, tt.columnType, got, tt.expected)
			}
		})
	}
}
