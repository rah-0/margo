package template

import (
	"encoding/base64"
	"errors"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rah-0/nabu"

	"github.com/rah-0/margo/conf"
	"github.com/rah-0/margo/db"
	"github.com/rah-0/margo/util"
)

var selectStarRegex = regexp.MustCompile(`(?i)select\s*\*`)

func CreateGoFileQueries() error {
	if conf.Args.QueriesPath == "" {
		return nil
	}
	fq, err := util.ReadFileAsString(conf.Args.QueriesPath)
	if err != nil {
		return nabu.FromError(err).WithArgs(conf.Args.QueriesPath).Log()
	}
	qs := SplitSQLQueries(fq)
	if err = CheckNoSelectStar(qs); err != nil {
		return nabu.FromError(err).WithArgs(fq).Log()
	}
	nqs := ExtractNamedQueries(qs)

	p := filepath.Join(conf.Args.OutputPath, db.NormalizeString(conf.Args.DBName), "queries.go")
	c := GetFileContentQueries(nqs)

	return util.WriteGoFile(p, c)
}

func GetFileContentQueries(nqs []conf.NamedQuery) string {
	t := "package " + db.NormalizeString(conf.Args.DBName) + "\n\n"
	t += GetCommentWarning()
	t += GetImportsQueries()
	t += GetVarsQueries(nqs)
	t += GetStructsQueries()
	t += GetGeneralFunctionsQueries()
	t += GetDBFunctionsQueries(nqs)

	return t
}

func GetImportsQueries() string {
	imports := "import (\n"
	imports += `"context"` + "\n"
	imports += `"database/sql"` + "\n"
	imports += `"encoding/base64"` + "\n"
	imports += `"sync"` + "\n"
	imports += ")\n\n"
	return imports
}

func SplitSQLQueries(content string) []string {
	var queries []string
	var sb strings.Builder
	inSingleQuote, inDoubleQuote := false, false

	for i := 0; i < len(content); i++ {
		c := content[i]

		if c == '\'' && !inDoubleQuote {
			inSingleQuote = !inSingleQuote
		} else if c == '"' && !inSingleQuote {
			inDoubleQuote = !inDoubleQuote
		}

		if c == ';' && !inSingleQuote && !inDoubleQuote {
			trimmed := strings.TrimSpace(sb.String())
			if trimmed != "" {
				queries = append(queries, trimmed)
			}
			sb.Reset()
			continue
		}

		sb.WriteByte(c)
	}

	trimmed := strings.TrimSpace(sb.String())
	if trimmed != "" {
		queries = append(queries, trimmed)
	}
	return queries
}

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

func GetVarsQueries(nqs []conf.NamedQuery) string {
	t := "var (\n"
	t += "db *sql.DB\n"
	t += "stmtMu sync.RWMutex\n"
	t += "stmtCache = make(map[string]*sql.Stmt)\n"
	t += "queries = map[string]*NamedQuery{\n"
	for _, q := range nqs {
		t += `"` + q.Name + `": {QueryEncoded: "` + q.QueryEncoded + `"},` + "\n"
	}
	t += "}\n"
	t += ")\n\n"
	return t
}

func GetGeneralFunctionsQueries() string {
	t := "func SetDB(x *sql.DB) error {\n"
	t += "db = x\n\n"
	t += "for _, q := range queries {\n"
	t += "b, err := base64.StdEncoding.DecodeString(q.QueryEncoded)\n"
	t += "if err != nil {\n"
	t += "return err\n"
	t += "}\n"
	t += "q.Query = string(b)\n"
	t += "}\n\n"
	t += "return nil\n"
	t += "}\n\n"

	t += "func getPreparedStmt(query string) (*sql.Stmt, error) {\n"
	t += "	stmtMu.RLock()\n"
	t += "	if stmt, ok := stmtCache[query]; ok {\n"
	t += "		stmtMu.RUnlock()\n"
	t += "		return stmt, nil\n"
	t += "	}\n"
	t += "	stmtMu.RUnlock()\n\n"
	t += "	stmtMu.Lock()\n"
	t += "	defer stmtMu.Unlock()\n"
	t += "	if stmt, ok := stmtCache[query]; ok {\n"
	t += "		return stmt, nil\n"
	t += "	}\n"
	t += "	stmt, err := db.Prepare(query)\n"
	t += "	if err != nil {\n"
	t += "		return nil, err\n"
	t += "	}\n"
	t += "	stmtCache[query] = stmt\n"
	t += "	return stmt, nil\n"
	t += "}\n\n"

	return t
}

func CheckNoSelectStar(queries []string) error {
	for i, q := range queries {
		normalized := strings.Join(strings.Fields(q), " ")
		if selectStarRegex.MatchString(normalized) {
			return nabu.FromError(errors.New("SELECT * is not allowed")).WithArgs(i, q).Log()
		}
	}
	return nil
}

func GetDBFunctionsQueries(nqs []conf.NamedQuery) string {
	genResultStruct := func(resultType string, fields []string) string {
		s := "type " + resultType + " struct {\n"
		for _, f := range fields {
			s += db.NormalizeString(f) + " string\n"
		}
		s += "}\n\n"
		return s
	}

	genQueryFunction := func(funcName, resultType, queryName string, hasParams, withContext bool, fields []string) string {
		s := "func " + funcName + "("
		if withContext {
			s += "ctx context.Context"
			if hasParams {
				s += ", args ...any"
			}
		} else if hasParams {
			s += "args ...any"
		}
		if len(fields) > 0 {
			s += ") ([]" + resultType + ", error) {\n"
		} else {
			s += ") (*sql.Rows, error) {\n"
		}

		s += "q := queries[\"" + queryName + "\"]\n"
		s += "stmt, err := getPreparedStmt(q.Query)\n"
		s += "if err != nil {\nreturn nil, err\n}\n"

		call := "stmt.Query"
		if withContext {
			call += "Context"
		}
		s += "rows, err := " + call
		if withContext || hasParams {
			s += "("
			if withContext && hasParams {
				s += "ctx, args..."
			} else if withContext {
				s += "ctx"
			} else {
				s += "args..."
			}
			s += ")"
		} else {
			s += "()"
		}
		s += "\n"

		s += "if err != nil {\nreturn nil, err\n}\n"

		if len(fields) == 0 {
			s += "return rows, nil\n"
			s += "}\n\n"
			return s
		}

		s += "defer rows.Close()\n"
		s += "var results []" + resultType + "\n"
		s += "for rows.Next() {\n"
		for _, f := range fields {
			s += "var ptr" + db.NormalizeString(f) + " *string\n"
		}
		s += "err := rows.Scan("
		for i, f := range fields {
			if i > 0 {
				s += ", "
			}
			s += "&ptr" + db.NormalizeString(f)
		}
		s += ")\n"
		s += "if err != nil {\nreturn results, err\n}\n"
		s += "x := " + resultType + "{}\n"
		for _, f := range fields {
			name := db.NormalizeString(f)
			s += "if ptr" + name + " != nil {\n"
			s += "x." + name + " = *ptr" + name + "\n"
			s += "} else {\n"
			s += "x." + name + " = \"\"\n"
			s += "}\n"
		}
		s += "results = append(results, x)\n"
		s += "}\n"
		s += "return results, nil\n"
		s += "}\n\n"
		return s
	}

	t := ""

	for _, nq := range nqs {
		funcName := "Query" + nq.Name
		resultType := funcName + "Result"
		hasParams := strings.Contains(nq.Query, "?")
		fields := ExtractSelectFields(nq.Query)

		if len(fields) > 0 {
			t += genResultStruct(resultType, fields)
		}

		t += genQueryFunction(funcName, resultType, nq.Name, hasParams, false, fields)
		t += genQueryFunction(funcName+"Context", resultType, nq.Name, hasParams, true, fields)
	}

	return t
}

func GetStructsQueries() string {
	t := "type NamedQuery struct {\n"
	t += "Name string\n"
	t += "Query string\n"
	t += "QueryEncoded string\n"
	t += "}\n\n"

	return t
}

func ExtractNamedQueries(rawQueries []string) []conf.NamedQuery {
	var out []conf.NamedQuery

	for _, raw := range rawQueries {
		var name string

		lines := strings.Split(raw, "\n")
		var cleanLines []string

		for _, line := range lines {
			trim := strings.TrimSpace(line)

			// Extract name tag if found
			if strings.HasPrefix(trim, "-- Name:") {
				name = strings.TrimSpace(strings.TrimPrefix(trim, "-- Name:"))
				continue
			}

			// Ignore line comments
			if strings.HasPrefix(trim, "--") || strings.HasPrefix(trim, "#") {
				continue
			}

			// Keep non-comment lines for final query
			cleanLines = append(cleanLines, line)
		}

		clean := StripSQLComments(strings.Join(cleanLines, "\n"))
		clean = strings.TrimSpace(clean)

		if clean == "" {
			continue
		}

		out = append(out, conf.NamedQuery{
			Name:         name,
			Query:        clean,
			QueryEncoded: base64.StdEncoding.EncodeToString([]byte(clean)),
		})
	}

	return out
}

func ExtractSelectFields(sql string) []string {
	sql = StripSQLComments(sql)
	original := strings.Join(strings.Fields(sql), " ") // normalize for parsing
	upper := strings.ToUpper(original)

	selectRe := regexp.MustCompile(`(?i)\bSELECT\b`)
	fromRe := regexp.MustCompile(`(?i)\bFROM\b`)

	selectLoc := selectRe.FindStringIndex(upper)
	fromLoc := fromRe.FindStringIndex(upper)

	if selectLoc == nil || fromLoc == nil || fromLoc[0] <= selectLoc[1] {
		return []string{}
	}

	// Use positions from normalized string to slice original
	fieldRegion := original[selectLoc[1]:fromLoc[0]]
	return ExtractFieldList(fieldRegion)
}

func ExtractFieldList(fragment string) []string {
	parts := strings.Split(fragment, ",")
	var out []string
	for _, raw := range parts {
		field := strings.TrimSpace(raw)
		if i := strings.Index(strings.ToLower(field), " as "); i != -1 {
			out = append(out, strings.Trim(field[i+4:], "` "))
		} else {
			segments := strings.Split(field, ".")
			out = append(out, strings.Trim(segments[len(segments)-1], "` "))
		}
	}
	return out
}
