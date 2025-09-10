package conf

type ResultMode string

const (
	ModeMany ResultMode = "many"
	ModeOne  ResultMode = "one"
	ModeExec ResultMode = "exec"
)
