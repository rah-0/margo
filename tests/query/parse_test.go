package query_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/rah-0/margo/errs"
	"github.com/rah-0/margo/query"
)

type stripSQLCommentsCase struct {
	name     string
	input    string
	expected string
}

func TestStripSQLComments(t *testing.T) {
	tests := []stripSQLCommentsCase{
		{
			"line comment at end",
			"SELECT 1; -- comment\nSELECT 2;",
			"SELECT 1; \nSELECT 2;",
		},
		{
			"block comment inline",
			"SELECT /* inline comment */ 1;",
			"SELECT  1;",
		},
		{
			"block comment multiline",
			"SELECT 1; /* comment\nacross lines */ SELECT 2;",
			"SELECT 1;  SELECT 2;",
		},
		{
			"quote with double dash",
			"SELECT '-- not a comment';",
			"SELECT '-- not a comment';",
		},
		{
			"quote with /* block */",
			`SELECT '/* not a comment */';`,
			`SELECT '/* not a comment */';`,
		},
		{
			"nested quotes and comments",
			`SELECT "abc"; -- comment`,
			`SELECT "abc"; `,
		},
		{
			"comment between queries",
			"SELECT 1; -- comment\n-- another\nSELECT 2;",
			"SELECT 1; \n\nSELECT 2;",
		},
		{
			"only comment",
			"-- full comment line\n",
			"\n",
		},
		{
			"no comments",
			"SELECT 1; SELECT 2;",
			"SELECT 1; SELECT 2;",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := query.StripSQLComments(tt.input)
			if got != tt.expected {
				t.Errorf("expected:\n%q\ngot:\n%q", tt.expected, got)
			}
		})
	}
}

func TestCheckNoSelectStar_Allowed(t *testing.T) {
	tests := []string{
		"SELECT id, name FROM users",
		"select count(*) from logs",
		"SELECT a.* FROM table a",
		"SELECT\nname\nFROM customers",
	}

	if err := query.CheckNoSelectStar(tests); err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
}

func TestCheckNoSelectStar_Disallowed(t *testing.T) {
	tests := [][]string{
		{"SELECT * FROM users"},
		{"select\n* from products"},
		{"Select     *     from items"},
	}

	for i, qset := range tests {
		if err := query.CheckNoSelectStar(qset); !errors.Is(err, errs.ErrSelectStarNotAllowed) || !strings.Contains(err.Error(), "index 0") {
			t.Errorf("test %d: expected SELECT * validation error for query 0, got %v", i, err)
		}
	}
}
