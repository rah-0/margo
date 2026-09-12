package errs

import "errors"

var (
	ErrGoModuleNotFound        = errors.New("go.mod: not found in any parent")
	ErrModuleDirectiveNotFound = errors.New("go.mod: module directive not found")
)
