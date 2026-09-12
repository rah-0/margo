//go:build margo_generated_benchmark

package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

type Alpha struct {
	ent.Schema
}

func (Alpha) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "alpha"},
	}
}

func (Alpha) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			NotEmpty().
			StorageKey("Uuid"),
		field.String("FirstInsert").
			StorageKey("FirstInsert").
			SchemaType(map[string]string{"mysql": "datetime(6)"}),
		field.String("LastUpdate").
			StorageKey("LastUpdate").
			SchemaType(map[string]string{"mysql": "datetime(6)"}),
		field.String("Animal").
			NotEmpty().
			StorageKey("Animal"),
		field.String("BigNumber").
			StorageKey("BigNumber"),
		field.String("TestField").
			Optional().
			StorageKey("test_field"),
	}
}
