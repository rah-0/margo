package conf

type Arguments struct {
	DBUser     string
	DBPassword string
	DBName     string
	DBIp       string
	DBPort     string
	OutputPath string
}

type TableField struct {
	Name       string
	DataType   string
	ColumnType string
	GOType     string
}
