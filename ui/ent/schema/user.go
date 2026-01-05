package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// User holds the schema definition for the User entity.
type User struct {
	ent.Schema
}

// Fields of the User.
func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("email").
			NotEmpty().
			Comment("User's email address"),
		field.String("name").
			Optional().
			Comment("User's display name"),
		field.String("password_hash").
			Optional().
			Sensitive().
			Comment("Bcrypt password hash (empty for OAuth users)"),
		field.Enum("role").
			Values("owner", "admin", "member", "viewer").
			Default("member").
			Comment("User's role within the org"),
		field.Enum("auth_provider").
			Values("local", "google", "github", "oidc").
			Default("local").
			Comment("Authentication provider"),
		field.String("auth_provider_id").
			Optional().
			Comment("ID from OAuth provider"),
		field.String("avatar_url").
			Optional().
			Comment("URL to user's avatar"),
		field.Bool("active").
			Default(true).
			Comment("Whether the user is active"),
		field.Time("last_login_at").
			Optional().
			Nillable().
			Comment("Last successful login"),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Comment("When the user was created"),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			Comment("When the user was last updated"),
	}
}

// Edges of the User.
func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("org", Org.Type).
			Ref("users").
			Unique().
			Comment("Organization the user belongs to"),
		edge.To("sessions", Session.Type).
			Comment("Active sessions for this user"),
	}
}

// Indexes of the User.
func (User) Indexes() []ent.Index {
	return []ent.Index{
		// Email must be unique within an org
		index.Fields("email").
			Edges("org").
			Unique(),
		index.Fields("auth_provider", "auth_provider_id"),
		index.Fields("active"),
	}
}
