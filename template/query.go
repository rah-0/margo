package template

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/rah-0/margo/conf"
	"github.com/rah-0/margo/db"
	"github.com/rah-0/margo/structs"
	"github.com/rah-0/margo/util"
)

// CreateGoFileQueries renders general queries and returns queries mapped to tables.
// Queries must already be loaded and parsed. The input slice is not modified.
func (r Renderer) CreateGoFileQueries(tns []string, queries []structs.NamedQuery) ([]structs.NamedQuery, error) {
	pathModuleOutput, err := util.GetGoModuleImportPath(r.OutputPath)
	if err != nil {
		return nil, fmt.Errorf("resolve output module %q: %w", r.OutputPath, err)
	}
	pathModuleOutput = path.Join(pathModuleOutput, db.NormalizeString(r.DBName))

	var general, mapped []structs.NamedQuery
	for _, nq := range queries {
		if nq.MapAs == "" {
			general = append(general, nq)
		} else {
			mapped = append(mapped, nq)
		}
	}
	if err := r.createGoFileErrors(); err != nil {
		return nil, fmt.Errorf("create shared errors file: %w", err)
	}

	p := filepath.Join(r.OutputPath, db.NormalizeString(r.DBName), "queries.go")
	content := r.GetFileContentQueries(pathModuleOutput, tns, general)
	return mapped, util.WriteGoFile(p, content)
}

func (r Renderer) GetFileContentQueries(pathModuleOutput string, tns []string, nqs []structs.NamedQuery) string {
	hasCustomQueries := len(nqs) > 0
	t := "package " + db.NormalizeString(r.DBName) + "\n\n"
	t += GetCommentWarning()
	t += getImportsQueries(pathModuleOutput, tns, nqs)
	t += GetVarsQueries(nqs)
	t += GetStructsQueries(hasCustomQueries)
	t += GetGeneralFunctionsQueries(tns, hasCustomQueries)
	t += GetDBFunctionsQueries(nqs)
	return t
}

func getImportsQueries(pathModuleOutput string, tns []string, nqs []structs.NamedQuery) string {
	hasMissingReturns, hasSingleRowQuery := false, false
	for _, nq := range nqs {
		mode := strings.ToLower(nq.Mode)
		if len(nq.Returns) == 0 && (mode == "" || mode == conf.ResultModeOne || mode == conf.ResultModeMany) {
			hasMissingReturns = true
		}
		if mode == conf.ResultModeOne && len(nq.Returns) > 0 {
			hasSingleRowQuery = true
		}
	}
	imports := "import (\n"
	imports += `"context"` + "\n"
	imports += `"database/sql"` + "\n"
	if len(nqs) > 0 {
		imports += `"encoding/base64"` + "\n"
	}
	if hasSingleRowQuery {
		imports += `"errors"` + "\n"
	}
	if hasMissingReturns {
		imports += `"fmt"` + "\n"
	}
	imports += `"sync"` + "\n\n"
	imports += `errs "` + path.Join(path.Dir(pathModuleOutput), "errs") + `"` + "\n"
	for _, tn := range tns {
		pathModuleTable := path.Join(pathModuleOutput, db.NormalizeString(tn))
		imports += `"` + pathModuleTable + `"` + "\n"
	}
	imports += ")\n\n"
	return imports
}

func GetVarsQueries(nqs []structs.NamedQuery) string {
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
	t += `return nil, errs.ErrDatabaseNotInitialized` + "\n"
	t += "}\n"
	t += "return db.Begin()\n"
	t += "}\n\n"
	t += "func NewCtxTx(ctx context.Context) (*sql.Tx, error) {\n"
	t += "if db == nil {\n"
	t += `return nil, errs.ErrDatabaseNotInitialized` + "\n"
	t += "}\n"
	t += "return db.BeginTx(ctx, nil)\n"
	t += "}\n\n"
	t += "func NewTxOpts(opts *sql.TxOptions) (*sql.Tx, error) {\n"
	t += "if db == nil {\n"
	t += `return nil, errs.ErrDatabaseNotInitialized` + "\n"
	t += "}\n"
	t += "return db.BeginTx(context.Background(), opts)\n"
	t += "}\n\n"
	t += "func NewCtxTxOpts(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {\n"
	t += "if db == nil {\n"
	t += `return nil, errs.ErrDatabaseNotInitialized` + "\n"
	t += "}\n"
	t += "return db.BeginTx(ctx, opts)\n"
	t += "}\n\n"

	t += "func getPreparedStmt(ctx context.Context, query string) (*sql.Stmt, error) {\n"
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
	t += "	if ctx == nil {\n"
	t += "		ctx = context.Background()\n"
	t += "	}\n"
	t += "	stmt, err := db.PrepareContext(ctx, query)\n"
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

func GetDBFunctionsQueries(nqs []structs.NamedQuery) string {
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

	genCore := func(nq structs.NamedQuery, mode string, fields []string, hasParams bool, innerType string) string {
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
			s += `qr = &Query` + nq.Name + `Result{Error: fmt.Errorf("named query ` + nq.Name + ` (ResultMode=` + mode + `): %w", errs.ErrMissingReturns)}` + "\n"
			s += "return\n"
			s += "}\n\n"
			return s
		}

		s := "func " + coreName + "(ctx context.Context, tx *sql.Tx, params *QueryParams) " + ret + " {\n"
		s += "qr = &Query" + nq.Name + "Result{}\n"
		s += "q := queries[\"" + nq.Name + "\"]\n"
		s += "base, err := getPreparedStmt(ctx, q.Query)\n"
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
			s += "defer func() { if cerr := rows.Close(); cerr != nil && qr.Error == nil { qr.Error = cerr } }()\n\n"
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

	genWrappers := func(nq structs.NamedQuery, mode string, fields []string, hasParams bool) string {
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
