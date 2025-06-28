package conf

import (
	"regexp"
)

var (
	Args         = Arguments{}
	BitSizeRegex = regexp.MustCompile(`bit\((\d+)\)`)
)
