package template

import (
	"reflect"
	"testing"
)

func TestCreateGoFileQueries(t *testing.T) {
	if err := CreateGoFileQueries(tableNames); err != nil {
		t.Fatal(err)
	}
}

func TestSplitSQLQueries(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "simple multiple queries",
			input:    "SELECT 1; SELECT 2;",
			expected: []string{"SELECT 1", "SELECT 2"},
		},
		{
			name:     "semicolon inside string",
			input:    "SELECT 'a;b;c'; SELECT 2;",
			expected: []string{"SELECT 'a;b;c'", "SELECT 2"},
		},
		{
			name:     "line comment with semicolon",
			input:    "SELECT 1; -- this is a comment;\nSELECT 2;",
			expected: []string{"SELECT 1", "SELECT 2"},
		},
		{
			name: "multiline query",
			input: `SELECT *
FROM table
WHERE col = 1;
SELECT 2;`,
			expected: []string{"SELECT *\nFROM table\nWHERE col = 1", "SELECT 2"},
		},
		{
			name:     "no semicolon",
			input:    "SELECT 1",
			expected: []string{"SELECT 1"},
		},
		{
			name:     "empty statements and whitespace",
			input:    " ; \nSELECT 1; ; SELECT 2; ",
			expected: []string{"SELECT 1", "SELECT 2"},
		},
		{
			name:     "quote with inline comment-looking string",
			input:    `SELECT '-- not a comment'; SELECT "/* also not a comment */";`,
			expected: []string{"SELECT '-- not a comment'", `SELECT "/* also not a comment */"`},
		},
		{
			name: "comment-only in between queries",
			input: `SELECT 1; -- comment
SELECT 2; /* ignored */ SELECT 3;`,
			expected: []string{"SELECT 1", "SELECT 2", "SELECT 3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := SplitSQLQueries(tt.input)
			named := ExtractNamedQueries(raw)

			var actual []string
			for _, nq := range named {
				actual = append(actual, nq.Query)
			}

			if !reflect.DeepEqual(actual, tt.expected) {
				t.Errorf("expected: %#v\ngot:      %#v", tt.expected, actual)
			}
		})
	}
}

func TestStripSQLComments(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
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
			got := StripSQLComments(tt.input)
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

	if err := CheckNoSelectStar(tests); err != nil {
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
		if err := CheckNoSelectStar(qset); err == nil {
			t.Errorf("Test %d: Expected error for SELECT *, got nil", i)
		}
	}
}

func TestExtractFieldList(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "simple fields",
			input:    "id, name, age",
			expected: []string{"id", "name", "age"},
		},
		{
			name:     "with aliases",
			input:    "u.id as user_id, u.name AS username, age",
			expected: []string{"user_id", "username", "age"},
		},
		{
			name:     "with backticks and alias",
			input:    "`u`.`id` as `UserID`, `u`.`name` as `UserName`",
			expected: []string{"UserID", "UserName"},
		},
		{
			name:     "with schema.table.field",
			input:    "db.users.id, users.name, age",
			expected: []string{"id", "name", "age"},
		},
		{
			name:     "spaces and tabs",
			input:    "  u.id\t, u.name  as  uname ,  age ",
			expected: []string{"id", "uname", "age"},
		},
		{
			name:     "no alias fallback",
			input:    "a.b.c, d",
			expected: []string{"c", "d"},
		},
		{
			name:     "empty input",
			input:    "",
			expected: []string{""},
		},
		{
			name:     "wildcard select",
			input:    "*",
			expected: []string{"*"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractFieldList(tt.input)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, got)
			}
		})
	}
}

func TestExtractSelectFields(t *testing.T) {
	tests := []struct {
		name     string
		sql      string
		expected []string
	}{
		{
			name:     "simple select",
			sql:      "SELECT id, name, age FROM users",
			expected: []string{"id", "name", "age"},
		},
		{
			name:     "with aliases",
			sql:      "SELECT u.id as uid, u.name AS uname FROM users",
			expected: []string{"uid", "uname"},
		},
		{
			name:     "with backticks",
			sql:      "SELECT `u`.`id`, `u`.`name` as `UserName` FROM `users`",
			expected: []string{"id", "UserName"},
		},
		{
			name: "select with linebreaks and tabs",
			sql: `SELECT
				u.id,
				u.name AS uname,
				age
			FROM users`,
			expected: []string{"id", "uname", "age"},
		},
		{
			name:     "select with excessive spacing",
			sql:      "   SELECT     id ,   name    , age   FROM   users   ",
			expected: []string{"id", "name", "age"},
		},
		{
			name:     "case insensitive keywords",
			sql:      "select id, name from users",
			expected: []string{"id", "name"},
		},
		{
			name:     "select with comment inside",
			sql:      "SELECT id, -- inline comment\nname FROM users",
			expected: []string{"id", "name"},
		},
		{
			name:     "invalid query (no FROM)",
			sql:      "SELECT id, name",
			expected: []string{},
		},
		{
			name:     "invalid query (no SELECT)",
			sql:      "UPDATE users SET name = 'x'",
			expected: []string{},
		},
		{
			name:     "wildcard select",
			sql:      "SELECT * FROM table",
			expected: []string{"*"},
		},
		{
			name:     "comment block between select and from",
			sql:      "SELECT id, /* name is important */ name FROM table",
			expected: []string{"id", "name"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := ExtractSelectFields(tt.sql)
			if !reflect.DeepEqual(actual, tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, actual)
			}
		})
	}
}
