package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Session holds the schema definition for the Session entity.
// A Session represents an active login session for a user.
type Session struct {
	ent.Schema
}

// Fields of the Session.
func (Session) Fields() []ent.Field {
	return []ent.Field{
		field.String("token").
			NotEmpty().
			Unique().
			Sensitive().
			Comment("Session token (hashed)"),
		field.String("ip_address").
			Optional().
			Comment("IP address of the client"),
		field.String("user_agent").
			Optional().
			Comment("User agent string"),
		field.Time("expires_at").
			Comment("When the session expires"),
		field.Time("last_active_at").
			Default(time.Now).
			Comment("Last activity time"),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Comment("When the session was created"),
	}
}

// Edges of the Session.
func (Session) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("sessions").
			Unique().
			Required().
			Comment("User who owns this session"),
	}
}

// Indexes of the Session.
func (Session) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("token"),
		index.Fields("expires_at"),
	}
}
