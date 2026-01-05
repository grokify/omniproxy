package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/grokify/omniproxy/ui/ent"
	"github.com/grokify/omniproxy/ui/ent/user"
)

var (
	// ErrOAuthFailed is returned when OAuth authentication fails.
	ErrOAuthFailed = errors.New("OAuth authentication failed")
	// ErrProviderNotConfigured is returned when OAuth provider is not configured.
	ErrProviderNotConfigured = errors.New("OAuth provider not configured")
)

// OAuthProvider represents an OAuth 2.0 provider configuration.
type OAuthProvider struct {
	Name         string
	ClientID     string
	ClientSecret string
	AuthURL      string
	TokenURL     string
	UserInfoURL  string
	Scopes       []string
	RedirectURL  string
}

// OAuthUser represents user info from an OAuth provider.
type OAuthUser struct {
	ID        string
	Email     string
	Name      string
	AvatarURL string
	Provider  string
}

// OAuthService handles OAuth authentication.
type OAuthService struct {
	client    *ent.Client
	auth      *Service
	providers map[string]*OAuthProvider
}

// NewOAuthService creates a new OAuth service.
func NewOAuthService(client *ent.Client, auth *Service) *OAuthService {
	return &OAuthService{
		client:    client,
		auth:      auth,
		providers: make(map[string]*OAuthProvider),
	}
}

// RegisterProvider registers an OAuth provider.
func (s *OAuthService) RegisterProvider(provider *OAuthProvider) {
	s.providers[provider.Name] = provider
}

// ConfigureGoogle configures Google OAuth.
func (s *OAuthService) ConfigureGoogle(clientID, clientSecret, redirectURL string) {
	s.RegisterProvider(&OAuthProvider{
		Name:         "google",
		ClientID:     clientID,
		ClientSecret: clientSecret,
		AuthURL:      "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL:     "https://oauth2.googleapis.com/token",
		UserInfoURL:  "https://www.googleapis.com/oauth2/v2/userinfo",
		Scopes:       []string{"email", "profile"},
		RedirectURL:  redirectURL,
	})
}

// ConfigureGitHub configures GitHub OAuth.
func (s *OAuthService) ConfigureGitHub(clientID, clientSecret, redirectURL string) {
	s.RegisterProvider(&OAuthProvider{
		Name:         "github",
		ClientID:     clientID,
		ClientSecret: clientSecret,
		AuthURL:      "https://github.com/login/oauth/authorize",
		TokenURL:     "https://github.com/login/oauth/access_token",
		UserInfoURL:  "https://api.github.com/user",
		Scopes:       []string{"read:user", "user:email"},
		RedirectURL:  redirectURL,
	})
}

// ConfigureOIDC configures a generic OIDC provider.
func (s *OAuthService) ConfigureOIDC(name, clientID, clientSecret, issuerURL, redirectURL string) error {
	// In a full implementation, we would fetch the .well-known/openid-configuration
	// For now, we require explicit URLs
	s.RegisterProvider(&OAuthProvider{
		Name:         name,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		AuthURL:      issuerURL + "/authorize",
		TokenURL:     issuerURL + "/token",
		UserInfoURL:  issuerURL + "/userinfo",
		Scopes:       []string{"openid", "email", "profile"},
		RedirectURL:  redirectURL,
	})
	return nil
}

// GetAuthURL returns the OAuth authorization URL for a provider.
func (s *OAuthService) GetAuthURL(providerName, state string) (string, error) {
	provider, ok := s.providers[providerName]
	if !ok {
		return "", ErrProviderNotConfigured
	}

	params := url.Values{}
	params.Set("client_id", provider.ClientID)
	params.Set("redirect_uri", provider.RedirectURL)
	params.Set("response_type", "code")
	params.Set("scope", strings.Join(provider.Scopes, " "))
	params.Set("state", state)

	return provider.AuthURL + "?" + params.Encode(), nil
}

// ExchangeCode exchanges an authorization code for user info and creates a session.
func (s *OAuthService) ExchangeCode(ctx context.Context, providerName, code string, orgID int, ipAddress, userAgent string) (*ent.Session, error) {
	provider, ok := s.providers[providerName]
	if !ok {
		return nil, ErrProviderNotConfigured
	}

	// Exchange code for token
	token, err := s.exchangeToken(provider, code)
	if err != nil {
		return nil, fmt.Errorf("token exchange failed: %w", err)
	}

	// Get user info
	oauthUser, err := s.getUserInfo(provider, token)
	if err != nil {
		return nil, fmt.Errorf("get user info failed: %w", err)
	}
	oauthUser.Provider = providerName

	// Find or create user
	u, err := s.findOrCreateUser(ctx, orgID, oauthUser)
	if err != nil {
		return nil, fmt.Errorf("find or create user failed: %w", err)
	}

	// Create session
	sess, err := s.auth.CreateSession(ctx, u.ID, ipAddress, userAgent)
	if err != nil {
		return nil, fmt.Errorf("create session failed: %w", err)
	}

	// Update last login time - non-critical, don't fail OAuth if this errors
	if err := s.client.User.UpdateOneID(u.ID).
		SetLastLoginAt(time.Now()).
		Exec(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "failed to update last login time: %v\n", err)
	}

	return sess, nil
}

// exchangeToken exchanges an authorization code for an access token.
func (s *OAuthService) exchangeToken(provider *OAuthProvider, code string) (string, error) {
	data := url.Values{}
	data.Set("client_id", provider.ClientID)
	data.Set("client_secret", provider.ClientSecret)
	data.Set("code", code)
	data.Set("redirect_uri", provider.RedirectURL)
	data.Set("grant_type", "authorization_code")

	req, err := http.NewRequest("POST", provider.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token request failed: %s", string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		Error       string `json:"error"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", err
	}

	if tokenResp.Error != "" {
		return "", fmt.Errorf("token error: %s", tokenResp.Error)
	}

	return tokenResp.AccessToken, nil
}

// getUserInfo fetches user info from the OAuth provider.
func (s *OAuthService) getUserInfo(provider *OAuthProvider, accessToken string) (*OAuthUser, error) {
	req, err := http.NewRequest("GET", provider.UserInfoURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("user info request failed: %s", string(body))
	}

	// Parse based on provider
	switch provider.Name {
	case "google":
		return parseGoogleUser(body)
	case "github":
		return parseGitHubUser(body)
	default:
		return parseOIDCUser(body)
	}
}

func parseGoogleUser(body []byte) (*OAuthUser, error) {
	var data struct {
		ID      string `json:"id"`
		Email   string `json:"email"`
		Name    string `json:"name"`
		Picture string `json:"picture"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}
	return &OAuthUser{
		ID:        data.ID,
		Email:     data.Email,
		Name:      data.Name,
		AvatarURL: data.Picture,
	}, nil
}

func parseGitHubUser(body []byte) (*OAuthUser, error) {
	var data struct {
		ID        int    `json:"id"`
		Login     string `json:"login"`
		Email     string `json:"email"`
		Name      string `json:"name"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}
	name := data.Name
	if name == "" {
		name = data.Login
	}
	return &OAuthUser{
		ID:        fmt.Sprintf("%d", data.ID),
		Email:     data.Email,
		Name:      name,
		AvatarURL: data.AvatarURL,
	}, nil
}

func parseOIDCUser(body []byte) (*OAuthUser, error) {
	var data struct {
		Sub     string `json:"sub"`
		Email   string `json:"email"`
		Name    string `json:"name"`
		Picture string `json:"picture"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}
	return &OAuthUser{
		ID:        data.Sub,
		Email:     data.Email,
		Name:      data.Name,
		AvatarURL: data.Picture,
	}, nil
}

// findOrCreateUser finds an existing user or creates a new one.
func (s *OAuthService) findOrCreateUser(ctx context.Context, orgID int, oauthUser *OAuthUser) (*ent.User, error) {
	// Try to find by provider ID
	u, err := s.client.User.Query().
		Where(
			user.AuthProviderEQ(user.AuthProvider(oauthUser.Provider)),
			user.AuthProviderID(oauthUser.ID),
		).
		Only(ctx)
	if err == nil {
		// Update user info from OAuth
		return s.client.User.UpdateOne(u).
			SetName(oauthUser.Name).
			SetAvatarURL(oauthUser.AvatarURL).
			Save(ctx)
	}
	if !ent.IsNotFound(err) {
		return nil, err
	}

	// Try to find by email
	u, err = s.client.User.Query().
		Where(user.Email(oauthUser.Email)).
		Only(ctx)
	if err == nil {
		// Link OAuth to existing user
		return s.client.User.UpdateOne(u).
			SetAuthProvider(user.AuthProvider(oauthUser.Provider)).
			SetAuthProviderID(oauthUser.ID).
			SetName(oauthUser.Name).
			SetAvatarURL(oauthUser.AvatarURL).
			Save(ctx)
	}
	if !ent.IsNotFound(err) {
		return nil, err
	}

	// Create new user
	return s.client.User.Create().
		SetOrgID(orgID).
		SetEmail(oauthUser.Email).
		SetName(oauthUser.Name).
		SetAuthProvider(user.AuthProvider(oauthUser.Provider)).
		SetAuthProviderID(oauthUser.ID).
		SetAvatarURL(oauthUser.AvatarURL).
		Save(ctx)
}
