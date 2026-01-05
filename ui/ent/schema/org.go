package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Org holds the schema definition for the Org entity.
// An Org represents an organization/tenant in multi-tenant mode.
type Org struct {
	ent.Schema
}

// Fields of the Org.
func (Org) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			NotEmpty().
			Comment("Organization display name"),
		field.String("slug").
			NotEmpty().
			Unique().
			Comment("URL-friendly unique identifier"),
		field.Enum("plan").
			Values("free", "team", "enterprise").
			Default("free").
			Comment("Subscription plan"),
		field.Int("traffic_retention_days").
			Default(30).
			Comment("How long to retain traffic data"),
		field.Int("max_proxies").
			Default(5).
			Comment("Maximum number of proxy instances"),
		field.Bool("active").
			Default(true).
			Comment("Whether the org is active"),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Comment("When the org was created"),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			Comment("When the org was last updated"),
	}
}

// Edges of the Org.
func (Org) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("users", User.Type).
			Comment("Users belonging to this org"),
		edge.To("proxies", Proxy.Type).
			Comment("Proxy instances belonging to this org"),
	}
}

// Indexes of the Org.
func (Org) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("slug"),
		index.Fields("active"),
	}
}
