package template

import (
	"github.com/rah-0/margo/conf"
	"github.com/rah-0/margo/db"
)

func GetImports(tfs []conf.TableField) string {
	importSet := map[string]struct{}{}

	for _, tf := range tfs {
		switch tf.GOType {
		case "time.Time":
			importSet[`"time"`] = struct{}{}
		case "uuid.UUID":
			importSet[`"github.com/google/uuid"`] = struct{}{}
		case "decimal.Decimal":
			importSet[`"github.com/shopspring/decimal"`] = struct{}{}
		}
	}

	if len(importSet) == 0 {
		return ""
	}

	imports := "import (\n"
	for imp := range importSet {
		imports += imp + "\n"
	}
	imports += ")"
	return imports
}

func GetStruct(rawTableName string, tfs []conf.TableField) string {
	t := "type " + db.NormalizeString(rawTableName) + " struct {\n"
	for _, tf := range tfs {
		t += db.NormalizeString(tf.Name) + " " + tf.GOType + "\n"
	}
	t += "}"
	return t
}
