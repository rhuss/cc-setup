package mcp

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rhuss/cc-mcp-setup/internal/config"
)

// tokenEndpointCache caches discovered token endpoints by origin.
var tokenEndpointCache sync.Map // map[string]string

// oauthRoundTripper injects an OAuth Bearer token into outgoing requests
// and transparently refreshes expired tokens using the stored refresh token.
type oauthRoundTripper struct {
	base     http.RoundTripper
	cred     *config.OAuthCredential
	entryKey string
	mu       sync.Mutex
}

func (rt *oauthRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.mu.Lock()
	token := rt.cred.AccessToken

	// Attempt refresh if the token is expired (with 30s grace window).
	if rt.cred.IsExpired(30*time.Second) && rt.cred.RefreshToken != "" {
		if refreshed := rt.refreshToken(); refreshed {
			token = rt.cred.AccessToken
		}
	}
	rt.mu.Unlock()

	// Clone the request before mutating headers.
	r := req.Clone(req.Context())
	r.Header.Set("Authorization", "Bearer "+token)
	return rt.base.RoundTrip(r)
}

// refreshToken attempts to refresh the OAuth access token. Returns true on
// success. On failure it logs nothing and returns false so the caller can
// proceed with the old (possibly expired) token for graceful degradation.
func (rt *oauthRoundTripper) refreshToken() bool {
	endpoint, err := discoverTokenEndpoint(rt.cred.ServerURL, rt.base)
	if err != nil {
		return false
	}

	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {rt.cred.RefreshToken},
		"client_id":     {rt.cred.ClientID},
	}

	resp, err := (&http.Client{Transport: rt.base}).Post(
		endpoint,
		"application/x-www-form-urlencoded",
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"` // seconds
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return false
	}
	if tokenResp.AccessToken == "" {
		return false
	}

	// Update in-memory credential.
	rt.cred.AccessToken = tokenResp.AccessToken
	if tokenResp.RefreshToken != "" {
		rt.cred.RefreshToken = tokenResp.RefreshToken
	}
	if tokenResp.ExpiresIn > 0 {
		rt.cred.ExpiresAt = time.Now().UnixMilli() + tokenResp.ExpiresIn*1000
	}

	// Best-effort write-back to disk.
	_ = config.SaveOAuthCredential(rt.entryKey, rt.cred)

	return true
}

// discoverTokenEndpoint finds the OAuth token endpoint for the given server URL
// using the RFC 8414 well-known metadata document.
func discoverTokenEndpoint(serverURL string, transport http.RoundTripper) (string, error) {
	origin, err := extractOrigin(serverURL)
	if err != nil {
		return "", err
	}

	// Check cache first.
	if cached, ok := tokenEndpointCache.Load(origin); ok {
		return cached.(string), nil
	}

	metaURL := origin + "/.well-known/oauth-authorization-server"
	client := &http.Client{
		Transport: transport,
		Timeout:   5 * time.Second,
	}

	resp, err := client.Get(metaURL) //nolint:noctx // best-effort discovery, no caller context needed
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", &tokenEndpointError{statusCode: resp.StatusCode}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var meta struct {
		TokenEndpoint string `json:"token_endpoint"`
	}
	if err := json.Unmarshal(body, &meta); err != nil {
		return "", err
	}
	if meta.TokenEndpoint == "" {
		return "", &tokenEndpointError{statusCode: 0}
	}

	// Resolve relative token endpoints against the origin.
	resolved := meta.TokenEndpoint
	if !strings.HasPrefix(resolved, "http://") && !strings.HasPrefix(resolved, "https://") {
		resolved = origin + "/" + strings.TrimPrefix(resolved, "/")
	}

	tokenEndpointCache.Store(origin, resolved)
	return resolved, nil
}

// extractOrigin returns the scheme://host[:port] portion of a URL.
func extractOrigin(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	return u.Scheme + "://" + u.Host, nil
}

// tokenEndpointError indicates a failure during token endpoint discovery.
type tokenEndpointError struct {
	statusCode int
}

func (e *tokenEndpointError) Error() string {
	if e.statusCode == 0 {
		return "token_endpoint not found in OAuth metadata"
	}
	return "OAuth metadata request failed: HTTP " + strconv.Itoa(e.statusCode)
}
