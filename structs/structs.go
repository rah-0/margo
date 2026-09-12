// Package structs defines data shared by MarGO's connection and generation packages.
package structs

// ConnectionOptions specifies an owned MariaDB connection. runner.Run requires
// all fields except Port and defaults an omitted Port to "3306" on a copy.
type ConnectionOptions struct {
	User     string
	Password string
	Host     string
	Port     string
	Database string
}

// TableField describes a database column used to generate bindings.
type TableField struct {
	Name       string
	DataType   string
	ColumnType string
}

// NamedQuery contains SQL and its binding-generation metadata.
type NamedQuery struct {
	Name         string
	Query        string
	QueryEncoded string

	Params  []string // from -- Params:
	Returns []string // from -- Returns:
	Mode    string   // from -- ResultMode: one|many|exec
	MapAs   string   // from -- MapAs:
}
