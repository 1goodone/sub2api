package schema

import (
	"fmt"

	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// APIKeyPackage holds the schema definition for user-owned API key templates.
type APIKeyPackage struct {
	ent.Schema
}

func (APIKeyPackage) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "api_key_packages"},
	}
}

func (APIKeyPackage) Mixin() []ent.Mixin {
	return []ent.Mixin{mixins.TimeMixin{}}
}

func (APIKeyPackage) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.String("name").
			MaxLen(100).
			NotEmpty(),
		field.String("cycle").
			MaxLen(10).
			Validate(func(value string) error {
				if value != "week" && value != "month" {
					return fmt.Errorf("cycle must be week or month")
				}
				return nil
			}),
		field.Float("amount_usd").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}),
		field.Bool("limit_5h_enabled").Default(false),
		field.Bool("limit_7d_enabled").Default(false),
	}
}

func (APIKeyPackage) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("api_key_packages").
			Field("user_id").
			Unique().
			Required(),
		edge.To("api_keys", APIKey.Type).
			Annotations(entsql.OnDelete(entsql.Restrict)),
	}
}

func (APIKeyPackage) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
	}
}
