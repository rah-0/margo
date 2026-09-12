package query

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"

	"github.com/rah-0/margo/conf"
	"github.com/rah-0/margo/errs"
	"github.com/rah-0/margo/structs"
	"github.com/rah-0/margo/util"
)

var selectStarRegex = regexp.MustCompile(`(?i)select\s*\*`)

// StripSQLComments removes SQL line and block comments while preserving quoted text.
func StripSQLComments(s string) string {
	var out strings.Builder
	inSingleQuote, inDoubleQuote := false, false
	inLineComment, inBlockComment := false, false

	for i := 0; i < len(s); i++ {
		c := s[i]
		next := byte(0)
		if i+1 < len(s) {
			next = s[i+1]
		}

		// Start of line comment --
		if !inSingleQuote && !inDoubleQuote && !inBlockComment && c == '-' && next == '-' {
			inLineComment = true
			i++ // Skip next '-'
			continue
		}
		// End of line comment
		if inLineComment {
			if c == '\n' {
				inLineComment = false
				out.WriteByte(c)
			}
			continue
		}

		// Start of block comment /*
		if !inSingleQuote && !inDoubleQuote && !inLineComment && c == '/' && next == '*' {
			inBlockComment = true
			i++ // Skip next '*'
			continue
		}
		// End of block comment */
		if inBlockComment {
			if c == '*' && next == '/' {
				inBlockComment = false
				i++ // Skip next '/'
			}
			continue
		}

		// Handle quoted strings (with escape)
		if !inLineComment && !inBlockComment {
			if c == '\'' && !inDoubleQuote {
				// Check for escaped single quote
				if inSingleQuote && i+1 < len(s) && s[i+1] == '\'' {
					out.WriteByte(c)
					i++ // Skip escaped quote
				} else {
					inSingleQuote = !inSingleQuote
				}
			} else if c == '"' && !inSingleQuote {
				// Check for escaped double quote
				if inDoubleQuote && i+1 < len(s) && s[i+1] == '"' {
					out.WriteByte(c)
					i++ // Skip escaped quote
				} else {
					inDoubleQuote = !inDoubleQuote
				}
			}
			out.WriteByte(c)
		}
	}
	return out.String()
}

// CheckNoSelectStar rejects SELECT * in queries with errs.ErrSelectStarNotAllowed.
func CheckNoSelectStar(queries []string) error {
	for i, q := range queries {
		normalized := strings.Join(strings.Fields(q), " ")
		if selectStarRegex.MatchString(normalized) {
			return fmt.Errorf("%w (index %d)", errs.ErrSelectStarNotAllowed, i)
		}
	}
	return nil
}

// ExtractNamedQuery parses SQL and its Params, Returns, ResultMode, and MapAs metadata.
func ExtractNamedQuery(content string, name string) structs.NamedQuery {
	var (
		params, returns []string
		mode            = conf.ResultModeMany
		cleanLines      []string
		mapAs           string
	)

	for _, line := range strings.Split(content, "\n") {
		trim := strings.TrimSpace(line)

		if v, ok := util.TrimPrefixCase(trim, "-- Params:"); ok {
			if v != "" {
				params = strings.Fields(v)
			}
			continue
		}
		if v, ok := util.TrimPrefixCase(trim, "-- Returns:"); ok {
			if v != "" {
				returns = strings.Fields(v)
			}
			continue
		}
		if v, ok := util.TrimPrefixCase(trim, "-- MapAs:"); ok {
			if v != "" {
				mapAs = v
			}
			continue
		}
		if v, ok := util.TrimPrefixCase(trim, "-- ResultMode:"); ok {
			mode = util.ParseResultMode(v)
			continue
		}
		// ignore other comment lines
		if strings.HasPrefix(trim, "--") || strings.HasPrefix(trim, "#") {
			continue
		}
		cleanLines = append(cleanLines, line)
	}

	clean := strings.TrimSpace(StripSQLComments(strings.Join(cleanLines, "\n")))

	return structs.NamedQuery{
		Name:         name,
		Query:        clean,
		QueryEncoded: base64.StdEncoding.EncodeToString([]byte(clean)),
		Params:       params,
		Returns:      returns,
		Mode:         mode, // "many" | "one" | "exec"
		MapAs:        mapAs,
	}
}
