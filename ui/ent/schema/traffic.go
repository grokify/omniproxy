package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Traffic holds the schema definition for the Traffic entity.
// A Traffic record represents a captured HTTP request/response pair.
type Traffic struct {
	ent.Schema
}

// Fields of the Traffic.
func (Traffic) Fields() []ent.Field {
	return []ent.Field{
		// Request fields
		field.String("method").
			NotEmpty().
			Comment("HTTP method (GET, POST, etc.)"),
		field.String("url").
			NotEmpty().
			Comment("Full request URL"),
		field.String("scheme").
			Comment("URL scheme (http/https)"),
		field.String("host").
			NotEmpty().
			Comment("Request host"),
		field.String("path").
			Comment("Request path"),
		field.String("query").
			Optional().
			Comment("Query string"),
		field.JSON("request_headers", map[string][]string{}).
			Optional().
			Comment("Request headers"),
		field.Bytes("request_body").
			Optional().
			Comment("Request body (may be truncated)"),
		field.Int64("request_body_size").
			Default(0).
			Comment("Original request body size"),
		field.Bool("request_is_binary").
			Default(false).
			Comment("Whether request body is binary"),
		field.String("content_type").
			Optional().
			Comment("Request Content-Type"),

		// Response fields
		field.Int("status_code").
			Comment("HTTP response status code"),
		field.String("status_text").
			Optional().
			Comment("HTTP status text"),
		field.JSON("response_headers", map[string][]string{}).
			Optional().
			Comment("Response headers"),
		field.Bytes("response_body").
			Optional().
			Comment("Response body (may be truncated)"),
		field.Int64("response_body_size").
			Default(0).
			Comment("Original response body size"),
		field.Bool("response_is_binary").
			Default(false).
			Comment("Whether response body is binary"),
		field.String("response_content_type").
			Optional().
			Comment("Response Content-Type"),

		// Timing fields
		field.Time("started_at").
			Comment("When the request started"),
		field.Float("duration_ms").
			Comment("Total duration in milliseconds"),
		field.Float("ttfb_ms").
			Optional().
			Nillable().
			Comment("Time to first byte in milliseconds"),

		// Metadata
		field.String("client_ip").
			Optional().
			Comment("Client IP address"),
		field.String("error").
			Optional().
			Comment("Error message if request failed"),
		field.JSON("tags", []string{}).
			Optional().
			Comment("User-defined tags"),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Comment("When the record was created"),
	}
}

// Edges of the Traffic.
func (Traffic) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("proxy", Proxy.Type).
			Ref("traffic").
			Unique().
			Required().
			Comment("Proxy that captured this traffic"),
	}
}

// Indexes of the Traffic.
func (Traffic) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("host"),
		index.Fields("path"),
		index.Fields("method"),
		index.Fields("status_code"),
		index.Fields("started_at"),
		index.Fields("host", "path"),
		index.Fields("method", "host", "path"),
	}
}
