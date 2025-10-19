package conf

type Arguments struct {
	DBUser      string
	DBPassword  string
	DBName      string
	DBIp        string
	DBPort      string
	OutputPath  string
	QueriesPath string
}

type TableField struct {
	Name       string
	DataType   string
	ColumnType string
}

type NamedQuery struct {
	Name         string
	Query        string
	QueryEncoded string

	Params  []string // from -- Params:
	Returns []string // from -- Returns:
	Mode    string   // from -- ResultMode: one|many|exec
	MapAs   string   // from -- MapAs:
}
