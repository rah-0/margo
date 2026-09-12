//go:build integration

package integration

import (
	"slices"
	"testing"

	margoDB "github.com/rah-0/margo/db"
)

func TestMariaDBContainer(t *testing.T) {
	database := StartMariaDB(t)

	if database.Settings.Host == "" || database.Settings.Port == "" {
		t.Fatalf(
			"expected a mapped MariaDB endpoint, got host %q and port %q",
			database.Settings.Host,
			database.Settings.Port,
		)
	}

	const countFixtureTables = `
		SELECT COUNT(*)
		FROM information_schema.tables
		WHERE table_schema = ?
		  AND table_name IN ('all_types', 'alpha', 'beta')`

	var tableCount int
	if err := database.DB.QueryRowContext(
		t.Context(),
		countFixtureTables,
		database.Settings.Database,
	).Scan(&tableCount); err != nil {
		t.Fatalf("count fixture tables: %v", err)
	}
	if tableCount != 3 {
		t.Fatalf("expected 3 fixture tables, got %d", tableCount)
	}

	tables, err := margoDB.GetDbTablesContext(t.Context(), database.DB, database.Settings.Database)
	if err != nil {
		t.Fatalf("inspect database tables: %v", err)
	}
	expectedTables := []string{"all_types", "alpha", "beta"}
	if !slices.Equal(tables, expectedTables) {
		t.Fatalf("expected tables %v, got %v", expectedTables, tables)
	}
	for _, table := range tables {
		fields, err := margoDB.GetDbTableFieldsContext(t.Context(), database.DB, database.Settings.Database, table)
		if err != nil {
			t.Fatalf("inspect fields for %s: %v", table, err)
		}
		if len(fields) == 0 {
			t.Fatalf("expected fields for %s", table)
		}
	}

	const name = "testcontainers"
	if _, err := database.DB.ExecContext(
		t.Context(),
		"INSERT INTO beta (name) VALUES (?)",
		name,
	); err != nil {
		t.Fatalf("insert row using UUID_v4 default: %v", err)
	}

	var uuid string
	if err := database.DB.QueryRowContext(
		t.Context(),
		"SELECT uuid FROM beta WHERE name = ?",
		name,
	).Scan(&uuid); err != nil {
		t.Fatalf("read generated UUID: %v", err)
	}
	if uuid == "" {
		t.Fatal("expected MariaDB to generate a UUID")
	}
}
