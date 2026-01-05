package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Proxy holds the schema definition for the Proxy entity.
// A Proxy represents a proxy instance configuration.
type Proxy struct {
	ent.Schema
}

// Fields of the Proxy.
func (Proxy) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			NotEmpty().
			Comment("Display name for this proxy"),
		field.String("slug").
			NotEmpty().
			Comment("URL-friendly identifier within org"),
		field.Enum("mode").
			Values("forward", "reverse", "transparent").
			Default("forward").
			Comment("Proxy operating mode"),
		field.Int("port").
			Default(8080).
			Comment("Port to listen on"),
		field.String("host").
			Default("127.0.0.1").
			Comment("Host to bind to"),
		field.Bool("mitm_enabled").
			Default(true).
			Comment("Enable HTTPS MITM interception"),
		field.JSON("skip_hosts", []string{}).
			Optional().
			Comment("Hosts to skip MITM for"),
		field.JSON("include_hosts", []string{}).
			Optional().
			Comment("Only capture traffic for these hosts"),
		field.JSON("exclude_hosts", []string{}).
			Optional().
			Comment("Exclude traffic from these hosts"),
		field.JSON("include_paths", []string{}).
			Optional().
			Comment("Only capture traffic for these paths"),
		field.JSON("exclude_paths", []string{}).
			Optional().
			Comment("Exclude traffic matching these paths"),
		field.String("upstream").
			Optional().
			Comment("Upstream proxy URL"),
		field.Bool("skip_binary").
			Default(true).
			Comment("Skip capturing binary content"),
		field.Bool("active").
			Default(true).
			Comment("Whether the proxy is active"),
		field.Time("last_started_at").
			Optional().
			Nillable().
			Comment("Last time the proxy was started"),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Comment("When the proxy was created"),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			Comment("When the proxy was last updated"),
	}
}

// Edges of the Proxy.
func (Proxy) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("org", Org.Type).
			Ref("proxies").
			Unique().
			Required().
			Comment("Organization this proxy belongs to"),
		edge.To("traffic", Traffic.Type).
			Comment("Traffic captured by this proxy"),
	}
}

// Indexes of the Proxy.
func (Proxy) Indexes() []ent.Index {
	return []ent.Index{
		// Slug must be unique within an org
		index.Fields("slug").
			Edges("org").
			Unique(),
		index.Fields("active"),
		index.Fields("mode"),
	}
}
