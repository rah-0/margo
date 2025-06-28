package template

import (
	"github.com/rah-0/margo/conf"
	"github.com/rah-0/margo/db"
)

func TemplateStruct(rawTableName string, tfs []conf.TableField) string {
	t := "type " + db.NormalizeTableName(rawTableName) + " struct {\n"
	for _, tf := range tfs {
		t += db.NormalizeTableName(tf.Name) + " " + tf.GOType + "\n"
	}
	t += "}"
	return t
}
