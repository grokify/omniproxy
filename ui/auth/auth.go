// Package auth provides authentication and session management.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/grokify/omniproxy/ui/ent"
	"github.com/grokify/omniproxy/ui/ent/session"
	"github.com/grokify/omniproxy/ui/ent/user"
	"golang.org/x/crypto/bcrypt"
)

var (
	// ErrInvalidCredentials is returned when email/password don't match.
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrUserNotFound is returned when user doesn't exist.
	ErrUserNotFound = errors.New("user not found")
	// ErrUserInactive is returned when user account is disabled.
	ErrUserInactive = errors.New("user account is inactive")
	// ErrSessionExpired is returned when session has expired.
	ErrSessionExpired = errors.New("session expired")
	// ErrSessionInvalid is returned when session token is invalid.
	ErrSessionInvalid = errors.New("invalid session")
)

// Service handles authentication operations.
type Service struct {
	client          *ent.Client
	sessionDuration time.Duration
	bcryptCost      int
}

// Config holds auth service configuration.
type Config struct {
	// SessionDuration is how long sessions last (default: 24 hours)
	SessionDuration time.Duration
	// BcryptCost is the bcrypt hashing cost (default: 12)
	BcryptCost int
}

// DefaultConfig returns default auth configuration.
func DefaultConfig() *Config {
	return &Config{
		SessionDuration: 24 * time.Hour,
		BcryptCost:      12,
	}
}

// NewService creates a new auth service.
func NewService(client *ent.Client, cfg *Config) *Service {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &Service{
		client:          client,
		sessionDuration: cfg.SessionDuration,
		bcryptCost:      cfg.BcryptCost,
	}
}

// HashPassword hashes a password using bcrypt.
func (s *Service) HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.bcryptCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// VerifyPassword checks if a password matches a hash.
func (s *Service) VerifyPassword(hash, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// Login authenticates a user with email and password.
func (s *Service) Login(ctx context.Context, orgID int, email, password, ipAddress, userAgent string) (*ent.Session, error) {
	// Find user by email in org
	u, err := s.client.User.Query().
		Where(
			user.Email(email),
			user.HasOrgWith( /* org.ID(orgID) */ ),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	// Check if user is active
	if !u.Active {
		return nil, ErrUserInactive
	}

	// Verify password
	if u.PasswordHash == "" || !s.VerifyPassword(u.PasswordHash, password) {
		return nil, ErrInvalidCredentials
	}

	// Create session
	sess, err := s.CreateSession(ctx, u.ID, ipAddress, userAgent)
	if err != nil {
		return nil, err
	}

	// Update last login time - non-critical, don't fail login if this errors
	if err := s.client.User.UpdateOneID(u.ID).
		SetLastLoginAt(time.Now()).
		Exec(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "failed to update last login time: %v\n", err)
	}

	return sess, nil
}

// CreateSession creates a new session for a user.
func (s *Service) CreateSession(ctx context.Context, userID int, ipAddress, userAgent string) (*ent.Session, error) {
	// Generate session token
	token, err := generateToken(32)
	if err != nil {
		return nil, err
	}

	// Hash the token for storage (we return the raw token to the client)
	hashedToken := hashToken(token)

	// Create session
	sess, err := s.client.Session.Create().
		SetToken(hashedToken).
		SetUserID(userID).
		SetIPAddress(ipAddress).
		SetUserAgent(userAgent).
		SetExpiresAt(time.Now().Add(s.sessionDuration)).
		SetLastActiveAt(time.Now()).
		Save(ctx)
	if err != nil {
		return nil, err
	}

	// Return session with raw token (client needs this)
	sess.Token = token
	return sess, nil
}

// ValidateSession validates a session token and returns the associated user.
func (s *Service) ValidateSession(ctx context.Context, token string) (*ent.User, *ent.Session, error) {
	hashedToken := hashToken(token)

	sess, err := s.client.Session.Query().
		Where(session.Token(hashedToken)).
		WithUser().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil, ErrSessionInvalid
		}
		return nil, nil, err
	}

	// Check expiration
	if time.Now().After(sess.ExpiresAt) {
		// Delete expired session - non-critical cleanup
		if err := s.client.Session.DeleteOne(sess).Exec(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "failed to delete expired session: %v\n", err)
		}
		return nil, nil, ErrSessionExpired
	}

	// Update last active time (async, don't block)
	go func() {
		if err := s.client.Session.UpdateOne(sess).
			SetLastActiveAt(time.Now()).
			Exec(context.Background()); err != nil {
			fmt.Fprintf(os.Stderr, "failed to update session last active time: %v\n", err)
		}
	}()

	u := sess.Edges.User
	if u == nil {
		return nil, nil, ErrSessionInvalid
	}

	if !u.Active {
		return nil, nil, ErrUserInactive
	}

	return u, sess, nil
}

// Logout invalidates a session.
func (s *Service) Logout(ctx context.Context, token string) error {
	hashedToken := hashToken(token)
	_, err := s.client.Session.Delete().
		Where(session.Token(hashedToken)).
		Exec(ctx)
	return err
}

// LogoutAll invalidates all sessions for a user.
func (s *Service) LogoutAll(ctx context.Context, userID int) error {
	_, err := s.client.Session.Delete().
		Where(session.HasUserWith(user.ID(userID))).
		Exec(ctx)
	return err
}

// CleanExpiredSessions removes all expired sessions.
func (s *Service) CleanExpiredSessions(ctx context.Context) (int, error) {
	return s.client.Session.Delete().
		Where(session.ExpiresAtLT(time.Now())).
		Exec(ctx)
}

// ExtendSession extends a session's expiration time.
func (s *Service) ExtendSession(ctx context.Context, token string) error {
	hashedToken := hashToken(token)
	return s.client.Session.Update().
		Where(session.Token(hashedToken)).
		SetExpiresAt(time.Now().Add(s.sessionDuration)).
		SetLastActiveAt(time.Now()).
		Exec(ctx)
}

// generateToken generates a cryptographically secure random token.
func generateToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// hashToken creates a SHA-256 hash of a token for secure storage.
func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}
