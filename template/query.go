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

func CreateGoFileQueries(tns []string) error {
	if conf.Args.QueriesPath == "" {
		return nil
	}

	pathModuleOutput, err := util.GetGoModuleImportPath(conf.Args.OutputPath)
	if err != nil {
		return nabu.FromError(err).WithArgs(conf.Args.OutputPath).Log()
	}
	pathModuleOutput = filepath.Join(pathModuleOutput, db.NormalizeString(conf.Args.DBName))

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
	c := GetFileContentQueries(pathModuleOutput, tns, nqs)

	return util.WriteGoFile(p, c)
}

func GetFileContentQueries(pathModuleOutput string, tns []string, nqs []conf.NamedQuery) string {
	t := "package " + db.NormalizeString(conf.Args.DBName) + "\n\n"
	t += GetCommentWarning()
	t += GetImportsQueries(pathModuleOutput, tns)
	t += GetVarsQueries(nqs)
	t += GetStructsQueries()
	t += GetGeneralFunctionsQueries(tns)
	t += GetDBFunctionsQueries(nqs)
	return t
}

func GetImportsQueries(pathModuleOutput string, tns []string) string {
	imports := "import (\n"
	imports += `"context"` + "\n"
	imports += `"database/sql"` + "\n"
	imports += `"encoding/base64"` + "\n"
	imports += `"errors"` + "\n"
	imports += `"sync"` + "\n\n"
	for _, tn := range tns {
		pathModuleTable := filepath.Join(pathModuleOutput, db.NormalizeString(tn))
		imports += `"` + pathModuleTable + `"` + "\n"
	}
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

func GetGeneralFunctionsQueries(tns []string) string {
	t := "func SetDB(x *sql.DB) error {\n"
	t += "db = x\n\n"
	t += "for _, q := range queries {\n"
	t += "b, err := base64.StdEncoding.DecodeString(q.QueryEncoded)\n"
	t += "if err != nil {\n"
	t += "return err\n"
	t += "}\n"
	t += "q.Query = string(b)\n"
	t += "}\n\n"
	for _, tn := range tns {
		t += db.NormalizeString(tn) + ".SetDB(x)\n"
	}
	t += "\nreturn nil\n"
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

	t += "func bindStmtCtxTx(base *sql.Stmt, ctx context.Context, tx *sql.Tx) (*sql.Stmt, bool) {\n"
	t += "	if tx == nil {\n"
	t += "		return base, false\n"
	t += "	}\n"
	t += "	if ctx != nil {\n"
	t += "		return tx.StmtContext(ctx, base), true\n"
	t += "	}\n"
	t += "	return tx.Stmt(base), true\n"
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
	t := ""

	genResultStruct := func(typeName string, fields []string) string {
		if len(fields) == 0 {
			return ""
		}
		s := "type " + typeName + " struct {\n"
		for _, f := range fields {
			s += db.NormalizeString(f) + " string\n"
		}
		s += "}\n\n"
		return s
	}

	genCore := func(nq conf.NamedQuery, mode string, fields []string, hasParams bool) string {
		coreName := "query" + nq.Name
		resType := "Query" + nq.Name + "Result"

		// signatures
		var ret string
		switch mode {
		case "exec":
			ret = "(res sql.Result, err error)"
		case "one":
			ret = "(_ *" + resType + ", err error)"
		default: // many
			ret = "(_ []" + resType + ", err error)"
		}

		// guard: enforce Returns for query modes
		if (mode == "many" || mode == "one") && len(fields) == 0 {
			s := "func " + coreName + "(ctx *context.Context, tx *sql.Tx"
			if hasParams {
				s += ", args ...any"
			}
			s += ") " + ret + " {\n"
			s += `err = fmt.Errorf("named query ` + nq.Name + ` requires -- Returns: for ResultMode=` + mode + `")` + "\n"
			s += "return\n"
			s += "}\n\n"
			return s
		}

		s := "func " + coreName + "(ctx *context.Context, tx *sql.Tx"
		if hasParams {
			s += ", args ...any"
		}
		s += ") " + ret + " {\n"
		s += "q := queries[\"" + nq.Name + "\"]\n"
		s += "base, err := getPreparedStmt(q.Query)\n"
		s += "if err != nil { return }\n\n"
		s += "var c context.Context\n"
		s += "if ctx != nil { c = *ctx }\n\n"
		s += "stmt, needClose := bindStmtCtxTx(base, c, tx)\n"
		s += "if needClose { defer func(){ if cerr := stmt.Close(); err == nil && cerr != nil { err = cerr } }() }\n\n"

		switch conf.ResultMode(mode) {
		case conf.ModeExec:
			if hasParams {
				s += "if ctx != nil { res, err = stmt.ExecContext(*ctx, args...) } else { res, err = stmt.Exec(args...) }\n"
			} else {
				s += "if ctx != nil { res, err = stmt.ExecContext(*ctx) } else { res, err = stmt.Exec() }\n"
			}
			s += "return\n"
			s += "}\n\n"
			return s

		case conf.ModeOne:
			// use QueryRow(…): no rows.Close needed
			for _, f := range fields {
				s += "var ptr" + db.NormalizeString(f) + " *string\n"
			}
			if hasParams {
				s += "if ctx != nil { err = stmt.QueryRowContext(*ctx, args...).Scan("
			} else {
				s += "if ctx != nil { err = stmt.QueryRowContext(*ctx).Scan("
			}
			for i, f := range fields {
				if i > 0 {
					s += ", "
				}
				s += "&ptr" + db.NormalizeString(f)
			}
			s += ") } else { err = stmt.QueryRow("
			if hasParams {
				s += "args..."
			}
			s += ").Scan("
			for i, f := range fields {
				if i > 0 {
					s += ", "
				}
				s += "&ptr" + db.NormalizeString(f)
			}
			s += ") }\n"
			s += "if errors.Is(err, sql.ErrNoRows) { return nil, nil }\n"
			s += "if err != nil { return }\n\n"
			s += "x := &" + resType + "{}\n"
			for _, f := range fields {
				fn := db.NormalizeString(f)
				s += "if ptr" + fn + " != nil { x." + fn + " = *ptr" + fn + " } else { x." + fn + " = \"\" }\n"
			}
			s += "return x, nil\n"
			s += "}\n\n"
			return s

		default: // many
			// materialize all rows
			s += "var rows *sql.Rows\n"
			if hasParams {
				s += "if ctx != nil { rows, err = stmt.QueryContext(*ctx, args...) } else { rows, err = stmt.Query(args...) }\n"
			} else {
				s += "if ctx != nil { rows, err = stmt.QueryContext(*ctx) } else { rows, err = stmt.Query() }\n"
			}
			s += "if err != nil { return }\n"
			s += "defer rows.Close()\n\n"
			s += "var out []" + resType + "\n"
			s += "for rows.Next() {\n"
			for _, f := range fields {
				s += "var ptr" + db.NormalizeString(f) + " *string\n"
			}
			s += "if err = rows.Scan("
			for i, f := range fields {
				if i > 0 {
					s += ", "
				}
				s += "&ptr" + db.NormalizeString(f)
			}
			s += "); err != nil { return }\n"
			s += "x := " + resType + "{}\n"
			for _, f := range fields {
				fn := db.NormalizeString(f)
				s += "if ptr" + fn + " != nil { x." + fn + " = *ptr" + fn + " } else { x." + fn + " = \"\" }\n"
			}
			s += "out = append(out, x)\n"
			s += "}\n"
			s += "if err = rows.Err(); err != nil { return }\n"
			s += "return out, nil\n"
			s += "}\n\n"
			return s
		}
	}

	genWrappers := func(nq conf.NamedQuery, mode string, fields []string, hasParams bool) string {
		core := "query" + nq.Name
		resType := "Query" + nq.Name + "Result"

		// params builder for wrappers
		params := func(withCtx, withTx bool) string {
			var ps []string
			if withCtx {
				ps = append(ps, "ctx context.Context")
			}
			if withTx {
				ps = append(ps, "tx *sql.Tx")
			}
			if hasParams {
				ps = append(ps, "args ...any")
			}
			return "(" + strings.Join(ps, ", ") + ")"
		}
		// call args to core
		coreArgs := func(withCtx, withTx bool) string {
			a := ""
			if withCtx {
				a += "&ctx"
			} else {
				a += "nil"
			}
			a += ", "
			if withTx {
				a += "tx"
			} else {
				a += "nil"
			}
			if hasParams {
				a += ", args..."
			}
			return a
		}

		var retMany, retOne, retExec string
		retMany = "([]" + resType + ", error)"
		retOne = "(*" + resType + ", error)"
		retExec = "(sql.Result, error)"

		namePrefix := ""
		switch mode {
		case "exec":
			namePrefix = "Exec" + nq.Name
		case "one":
			namePrefix = "Query" + nq.Name
		default:
			namePrefix = "Query" + nq.Name
		}

		s := ""

		switch mode {
		case "exec":
			s += "func " + namePrefix + params(false, false) + " " + retExec + " { return " + core + "(" + coreArgs(false, false) + ") }\n"
			s += "func " + namePrefix + "Ctx" + params(true, false) + " " + retExec + " { return " + core + "(" + coreArgs(true, false) + ") }\n"
			s += "func " + namePrefix + "Tx" + params(false, true) + " " + retExec + " { return " + core + "(" + coreArgs(false, true) + ") }\n"
			s += "func " + namePrefix + "CtxTx" + params(true, true) + " " + retExec + " { return " + core + "(" + coreArgs(true, true) + ") }\n\n"
		case "one":
			s += "func " + namePrefix + params(false, false) + " " + retOne + " { return " + core + "(" + coreArgs(false, false) + ") }\n"
			s += "func " + namePrefix + "Ctx" + params(true, false) + " " + retOne + " { return " + core + "(" + coreArgs(true, false) + ") }\n"
			s += "func " + namePrefix + "Tx" + params(false, true) + " " + retOne + " { return " + core + "(" + coreArgs(false, true) + ") }\n"
			s += "func " + namePrefix + "CtxTx" + params(true, true) + " " + retOne + " { return " + core + "(" + coreArgs(true, true) + ") }\n\n"
		default:
			s += "func " + namePrefix + params(false, false) + " " + retMany + " { return " + core + "(" + coreArgs(false, false) + ") }\n"
			s += "func " + namePrefix + "Ctx" + params(true, false) + " " + retMany + " { return " + core + "(" + coreArgs(true, false) + ") }\n"
			s += "func " + namePrefix + "Tx" + params(false, true) + " " + retMany + " { return " + core + "(" + coreArgs(false, true) + ") }\n"
			s += "func " + namePrefix + "CtxTx" + params(true, true) + " " + retMany + " { return " + core + "(" + coreArgs(true, true) + ") }\n\n"
		}
		return s
	}

	for _, nq := range nqs {
		mode := strings.ToLower(string(nq.Mode))
		if mode == "" {
			mode = "many"
		}
		fields := nq.Returns
		hasParams := strings.Contains(nq.Query, "?")

		// struct for query modes
		if (mode == "many" || mode == "one") && len(fields) > 0 {
			t += genResultStruct("Query"+nq.Name+"Result", fields)
		}
		// core + 4 wrappers
		t += genCore(nq, mode, fields, hasParams)
		t += genWrappers(nq, mode, fields, hasParams)
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
	out := make([]conf.NamedQuery, 0, len(rawQueries))

	for _, raw := range rawQueries {
		var (
			name            string
			params, returns []string
			mode            = "many"
			useTx, useCtx   bool
			cleanLines      []string
		)

		for _, line := range strings.Split(raw, "\n") {
			trim := strings.TrimSpace(line)

			if v, ok := util.TrimPrefixCase(trim, "-- Name:"); ok {
				name = v
				continue
			}
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
			if v, ok := util.TrimPrefixCase(trim, "-- ResultMode:"); ok {
				mode = util.ParseResultMode(v)
				continue
			}
			if strings.HasPrefix(trim, "-- Transaction") {
				useTx = true
				continue
			}
			if strings.HasPrefix(trim, "-- Context") {
				useCtx = true
				continue
			}
			// ignore other comment lines
			if strings.HasPrefix(trim, "--") || strings.HasPrefix(trim, "#") {
				continue
			}
			cleanLines = append(cleanLines, line)
		}

		clean := strings.TrimSpace(StripSQLComments(strings.Join(cleanLines, "\n")))
		if clean == "" {
			continue
		}

		out = append(out, conf.NamedQuery{
			Name:         name,
			Query:        clean,
			QueryEncoded: base64.StdEncoding.EncodeToString([]byte(clean)),
			Params:       params,
			Returns:      returns,
			Mode:         conf.ResultMode(mode), // "many" | "one" | "exec"
			UseTx:        useTx,
			UseCtx:       useCtx,
		})
	}

	return out
}
