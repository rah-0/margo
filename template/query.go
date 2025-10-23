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

func CreateGoFileQueries(tns []string) ([]conf.NamedQuery, error) {
	pathModuleOutput, err := util.GetGoModuleImportPath(conf.Args.OutputPath)
	if err != nil {
		return []conf.NamedQuery{}, nabu.FromError(err).WithArgs(conf.Args.OutputPath).Log()
	}
	pathModuleOutput = filepath.Join(pathModuleOutput, db.NormalizeString(conf.Args.DBName))

	nqsGeneral := []conf.NamedQuery{}
	nqsTableSpecific := []conf.NamedQuery{}

	// Only process queries if a queries path is provided
	if conf.Args.QueriesPath != "" {
		// Read all .sql files from directory
		sqlFiles, err := util.GetSQLFilesInDir(conf.Args.QueriesPath)
		if err != nil {
			return []conf.NamedQuery{}, nabu.FromError(err).WithArgs(conf.Args.QueriesPath).Log()
		}

		for _, sqlFile := range sqlFiles {
			content, err := util.ReadFileAsString(sqlFile)
			if err != nil {
				return []conf.NamedQuery{}, nabu.FromError(err).WithArgs(sqlFile).Log()
			}
			if err = CheckNoSelectStar([]string{content}); err != nil {
				return []conf.NamedQuery{}, nabu.FromError(err).WithArgs(sqlFile).Log()
			}

			// Extract query name from filename (without .sql extension)
			baseName := filepath.Base(sqlFile)
			queryName := strings.TrimSuffix(baseName, filepath.Ext(baseName))

			// Extract query with name from filename
			nq := ExtractNamedQuery(content, queryName)
			if nq.MapAs == "" {
				nqsGeneral = append(nqsGeneral, nq)
			} else {
				nqsTableSpecific = append(nqsTableSpecific, nq)
			}
		}
	}

	// Always generate queries.go, even with no custom queries
	p := filepath.Join(conf.Args.OutputPath, db.NormalizeString(conf.Args.DBName), "queries.go")
	c := GetFileContentQueries(pathModuleOutput, tns, nqsGeneral)

	return nqsTableSpecific, util.WriteGoFile(p, c)
}

func GetFileContentQueries(pathModuleOutput string, tns []string, nqs []conf.NamedQuery) string {
	hasCustomQueries := len(nqs) > 0
	t := "package " + db.NormalizeString(conf.Args.DBName) + "\n\n"
	t += GetCommentWarning()
	t += GetImportsQueries(pathModuleOutput, tns, hasCustomQueries)
	t += GetVarsQueries(nqs)
	t += GetStructsQueries(hasCustomQueries)
	t += GetGeneralFunctionsQueries(tns, hasCustomQueries)
	t += GetDBFunctionsQueries(nqs)
	return t
}

func GetImportsQueries(pathModuleOutput string, tns []string, hasCustomQueries bool) string {
	imports := "import (\n"
	imports += `"context"` + "\n"
	imports += `"database/sql"` + "\n"
	if hasCustomQueries {
		imports += `"encoding/base64"` + "\n"
	}
	imports += `"errors"` + "\n"
	imports += `"sync"` + "\n\n"
	for _, tn := range tns {
		pathModuleTable := filepath.Join(pathModuleOutput, db.NormalizeString(tn))
		imports += `"` + pathModuleTable + `"` + "\n"
	}
	imports += ")\n\n"
	return imports
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
	if len(nqs) > 0 {
		t += "queries = map[string]*NamedQuery{\n"
		for _, q := range nqs {
			t += `"` + q.Name + `": {QueryEncoded: "` + q.QueryEncoded + `"},` + "\n"
		}
		t += "}\n"
	}
	t += ")\n\n"
	return t
}

func GetGeneralFunctionsQueries(tns []string, hasCustomQueries bool) string {
	t := "func SetDB(x *sql.DB) error {\n"
	t += "db = x\n\n"
	if hasCustomQueries {
		t += "for _, q := range queries {\n"
		t += "b, err := base64.StdEncoding.DecodeString(q.QueryEncoded)\n"
		t += "if err != nil {\n"
		t += "return err\n"
		t += "}\n"
		t += "q.Query = string(b)\n"
		t += "}\n\n"
	}
	for _, tn := range tns {
		t += "if err := " + db.NormalizeString(tn) + ".SetDB(x); err != nil {\n"
		t += "return err\n"
		t += "}\n"
	}
	t += "\nreturn nil\n"
	t += "}\n\n"

	t += "func NewTx() (*sql.Tx, error) {\n"
	t += "if db == nil {\n"
	t += `return nil, errors.New("db not initialized")` + "\n"
	t += "}\n"
	t += "return db.Begin()\n"
	t += "}\n\n"
	t += "func NewCtxTx(ctx context.Context) (*sql.Tx, error) {\n"
	t += "if db == nil {\n"
	t += `return nil, errors.New("db not initialized")` + "\n"
	t += "}\n"
	t += "return db.BeginTx(ctx, nil)\n"
	t += "}\n\n"
	t += "func NewTxOpts(opts *sql.TxOptions) (*sql.Tx, error) {\n"
	t += "if db == nil {\n"
	t += `return nil, errors.New("db not initialized")` + "\n"
	t += "}\n"
	t += "return db.BeginTx(context.Background(), opts)\n"
	t += "}\n\n"
	t += "func NewCtxTxOpts(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {\n"
	t += "if db == nil {\n"
	t += `return nil, errors.New("db not initialized")` + "\n"
	t += "}\n"
	t += "return db.BeginTx(ctx, opts)\n"
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

	genCore := func(nq conf.NamedQuery, mode string, fields []string, hasParams bool, innerType string) string {
		coreName := "query" + nq.Name
		resType := innerType
		if resType == "" {
			resType = "Query" + nq.Name + "ResultInner"
		}

		// signatures - all return *QueryResult now
		ret := "(qr *Query" + nq.Name + "Result)"

		// guard: enforce Returns for query modes
		if (mode == conf.ResultModeMany || mode == conf.ResultModeOne) && len(fields) == 0 {
			s := "func " + coreName + "(ctx context.Context, tx *sql.Tx, params *QueryParams) " + ret + " {\n"
			s += `qr = &Query` + nq.Name + `Result{Error: fmt.Errorf("named query ` + nq.Name + ` requires -- Returns: for ResultMode=` + mode + `")}` + "\n"
			s += "return\n"
			s += "}\n\n"
			return s
		}

		s := "func " + coreName + "(ctx context.Context, tx *sql.Tx, params *QueryParams) " + ret + " {\n"
		s += "qr = &Query" + nq.Name + "Result{}\n"
		s += "q := queries[\"" + nq.Name + "\"]\n"
		s += "base, err := getPreparedStmt(q.Query)\n"
		s += "if err != nil { qr.Error = err; return }\n\n"
		s += "stmt, needClose := bindStmtCtxTx(base, ctx, tx)\n"
		s += "if needClose { defer func(){ if cerr := stmt.Close(); err == nil && cerr != nil { qr.Error = cerr } }() }\n\n"

		switch mode {
		case conf.ResultModeExec:
			s += "var res sql.Result\n"
			if hasParams {
				s += "if ctx != nil { res, err = stmt.ExecContext(ctx, params.Params...) } else { res, err = stmt.Exec(params.Params...) }\n"
			} else {
				s += "if ctx != nil { res, err = stmt.ExecContext(ctx) } else { res, err = stmt.Exec() }\n"
			}
			s += "qr.Result = res\n"
			s += "qr.Error = err\n"
			s += "return\n"
			s += "}\n\n"
			return s

		case conf.ResultModeOne:
			// use QueryRow(…): no rows.Close needed
			for _, f := range fields {
				s += "var ptr" + db.NormalizeString(f) + " *string\n"
			}
			if hasParams {
				s += "if ctx != nil { err = stmt.QueryRowContext(ctx, params.Params...).Scan("
			} else {
				s += "if ctx != nil { err = stmt.QueryRowContext(ctx).Scan("
			}
			for i, f := range fields {
				if i > 0 {
					s += ", "
				}
				s += "&ptr" + db.NormalizeString(f)
			}
			s += ") } else { err = stmt.QueryRow("
			if hasParams {
				s += "params.Params..."
			}
			s += ").Scan("
			for i, f := range fields {
				if i > 0 {
					s += ", "
				}
				s += "&ptr" + db.NormalizeString(f)
			}
			s += ") }\n"
			s += "if errors.Is(err, sql.ErrNoRows) { return }\n"
			s += "if err != nil { qr.Error = err; return }\n\n"
			s += "x := &" + resType + "{}\n"
			for _, f := range fields {
				fn := db.NormalizeString(f)
				s += "if ptr" + fn + " != nil { x." + fn + " = *ptr" + fn + " } else { x." + fn + " = \"\" }\n"
			}
			s += "qr.Entity = x\n"
			s += "qr.Exists = true\n"
			s += "return\n"
			s += "}\n\n"
			return s

		default: // many
			// materialize all rows
			s += "var rows *sql.Rows\n"
			if hasParams {
				s += "if ctx != nil { rows, err = stmt.QueryContext(ctx, params.Params...) } else { rows, err = stmt.Query(params.Params...) }\n"
			} else {
				s += "if ctx != nil { rows, err = stmt.QueryContext(ctx) } else { rows, err = stmt.Query() }\n"
			}
			s += "if err != nil { qr.Error = err; return }\n"
			s += "defer rows.Close()\n\n"
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
			s += "); err != nil { qr.Error = err; return }\n"
			s += "x := " + resType + "{}\n"
			for _, f := range fields {
				fn := db.NormalizeString(f)
				s += "if ptr" + fn + " != nil { x." + fn + " = *ptr" + fn + " } else { x." + fn + " = \"\" }\n"
			}
			s += "qr.Entities = append(qr.Entities, &x)\n"
			s += "}\n"
			s += "if err = rows.Err(); err != nil { qr.Error = err; return }\n"
			s += "return\n"
			s += "}\n\n"
			return s
		}
	}

	genWrappers := func(nq conf.NamedQuery, mode string, fields []string, hasParams bool) string {
		core := "query" + nq.Name
		resType := "Query" + nq.Name + "Result"

		// params builder for wrappers - conditionally include params based on hasParams
		params := func(withCtx, withTx bool) string {
			var ps []string
			if withCtx {
				ps = append(ps, "ctx context.Context")
			}
			if withTx {
				ps = append(ps, "tx *sql.Tx")
			}
			if hasParams {
				ps = append(ps, "params *QueryParams")
			}
			return "(" + strings.Join(ps, ", ") + ")"
		}
		// call args to core
		coreArgs := func(withCtx, withTx bool) string {
			a := ""
			if withCtx {
				a += "ctx"
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
				a += ", params"
			} else {
				a += ", nil"
			}
			return a
		}

		ret := "*" + resType

		namePrefix := ""
		switch mode {
		case conf.ResultModeExec:
			namePrefix = "Exec" + nq.Name
		case conf.ResultModeOne:
			namePrefix = "Query" + nq.Name
		default: // many
			namePrefix = "Query" + nq.Name
		}

		s := ""
		s += "func " + namePrefix + params(false, false) + " " + ret + " { return " + core + "(" + coreArgs(false, false) + ") }\n"
		s += "func " + namePrefix + "Ctx" + params(true, false) + " " + ret + " { return " + core + "(" + coreArgs(true, false) + ") }\n"
		s += "func " + namePrefix + "Tx" + params(false, true) + " " + ret + " { return " + core + "(" + coreArgs(false, true) + ") }\n"
		s += "func " + namePrefix + "CtxTx" + params(true, true) + " " + ret + " { return " + core + "(" + coreArgs(true, true) + ") }\n\n"
		return s
	}

	for _, nq := range nqs {
		mode := strings.ToLower(string(nq.Mode))
		if mode == "" {
			mode = conf.ResultModeMany
		}
		fields := nq.Returns
		hasParams := strings.Contains(nq.Query, "?")

		// struct for query modes - generate inner result struct if needed
		innerType := ""
		if (mode == conf.ResultModeMany || mode == conf.ResultModeOne) && len(fields) > 0 {
			innerType = "Query" + nq.Name + "ResultInner"
			t += genResultStruct(innerType, fields)
		}

		// generate QueryResult wrapper struct
		t += "type Query" + nq.Name + "Result struct {\n"
		if innerType != "" {
			if mode == conf.ResultModeOne {
				t += "Entity *" + innerType + "\n"
			} else {
				t += "Entities []*" + innerType + "\n"
			}
		}
		t += "Error error\n"
		t += "Result sql.Result\n"
		if mode == conf.ResultModeOne {
			t += "Exists bool\n"
		}
		t += "}\n\n"

		// core + 4 wrappers
		t += genCore(nq, mode, fields, hasParams, innerType)
		t += genWrappers(nq, mode, fields, hasParams)
	}

	return t
}

func GetStructsQueries(hasCustomQueries bool) string {
	if !hasCustomQueries {
		return ""
	}

	t := "type NamedQuery struct {\n"
	t += "Name string\n"
	t += "Query string\n"
	t += "QueryEncoded string\n"
	t += "}\n\n"

	t += "type QueryParams struct {\n"
	t += "Params []any\n"
	t += "}\n\n"

	t += "func NewQueryParams() *QueryParams {\n"
	t += "return &QueryParams{}\n"
	t += "}\n\n"

	t += "func (qp *QueryParams) WithParams(params ...any) *QueryParams {\n"
	t += "qp.Params = params\n"
	t += "return qp\n"
	t += "}\n\n"

	return t
}

func ExtractNamedQuery(content string, name string) conf.NamedQuery {
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

	return conf.NamedQuery{
		Name:         name,
		Query:        clean,
		QueryEncoded: base64.StdEncoding.EncodeToString([]byte(clean)),
		Params:       params,
		Returns:      returns,
		Mode:         mode, // "many" | "one" | "exec"
		MapAs:        mapAs,
	}
}
