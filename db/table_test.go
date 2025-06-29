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
