package mcp

import (
	"context"
	"crypto/tls"
	"net/http"
	"os/exec"
	"strings"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/cc-deck/cc-setup/internal/config"
)

// HealthStatus represents the reachability state of an MCP server.
type HealthStatus int

const (
	HealthUnknown      HealthStatus = iota
	HealthOK                                // server connected and initialized
	HealthAuthRequired                      // server reachable but requires auth (OAuth, etc.)
	HealthError                             // server unreachable or protocol error
)

// ToolInfo holds display-relevant metadata for a single MCP tool.
type ToolInfo struct {
	Name            string
	Description     string
	ReadOnlyHint    bool
	DestructiveHint bool
	HasAnnotations  bool // true when the server explicitly provided annotations
}

// clientImpl is the Implementation sent during initialize.
var clientImpl = &sdkmcp.Implementation{
	Name:    "cc-setup",
	Version: "0.1.0",
}

// HealthResult holds the outcome of a health check.
type HealthResult struct {
	Status HealthStatus
	Err    error // non-nil when Status is HealthError
}

// CheckHealth connects to the MCP server defined by serverDef and returns
// its health result. The caller should pass a context with an appropriate
// timeout (e.g. 5s). serverName is used to look up OAuth credentials.
func CheckHealth(ctx context.Context, serverName string, serverDef map[string]any) HealthResult {
	transport, err := buildTransport(serverName, serverDef)
	if err != nil {
		return HealthResult{Status: HealthError, Err: err}
	}
	client := sdkmcp.NewClient(clientImpl, nil)
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		if isAuthError(err) {
			return HealthResult{Status: HealthAuthRequired, Err: err}
		}
		return HealthResult{Status: HealthError, Err: err}
	}
	_ = session.Close()
	return HealthResult{Status: HealthOK}
}

// isAuthError checks whether the error indicates the server is reachable but
// requires authentication the CLI cannot provide (e.g. OAuth).
func isAuthError(err error) bool {
	msg := err.Error()
	for _, indicator := range []string{
		"Unauthorized",
		"Forbidden",
		"401",
		"403",
	} {
		if strings.Contains(msg, indicator) {
			return true
		}
	}
	return false
}

// ListTools connects to the MCP server, discovers all available tools, and
// returns their metadata. The caller should pass a context with an appropriate
// timeout (e.g. 15s). serverName is used to look up OAuth credentials.
func ListTools(ctx context.Context, serverName string, serverDef map[string]any) ([]ToolInfo, error) {
	transport, err := buildTransport(serverName, serverDef)
	if err != nil {
		return nil, err
	}
	client := sdkmcp.NewClient(clientImpl, nil)
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, err
	}
	defer session.Close()

	var tools []ToolInfo
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			return nil, err
		}
		info := ToolInfo{
			Name:        tool.Name,
			Description: tool.Description,
		}
		if tool.Annotations != nil {
			info.HasAnnotations = true
			info.ReadOnlyHint = tool.Annotations.ReadOnlyHint
			if tool.Annotations.DestructiveHint != nil {
				info.DestructiveHint = *tool.Annotations.DestructiveHint
			}
		}
		tools = append(tools, info)
	}
	return tools, nil
}

// buildTransport creates an appropriate MCP transport based on the server
// definition's "type" field.
func buildTransport(serverName string, serverDef map[string]any) (sdkmcp.Transport, error) {
	stype, _ := serverDef["type"].(string)
	switch stype {
	case "stdio":
		return buildStdioTransport(serverDef), nil
	case "http":
		return buildStreamableTransport(serverName, serverDef), nil
	case "sse":
		return buildSSETransport(serverName, serverDef), nil
	default:
		return nil, &UnsupportedTransportError{Type: stype}
	}
}

// UnsupportedTransportError is returned when the server type is not recognized.
type UnsupportedTransportError struct {
	Type string
}

func (e *UnsupportedTransportError) Error() string {
	return "unsupported transport type: " + e.Type
}

func buildStdioTransport(serverDef map[string]any) *sdkmcp.CommandTransport {
	command, _ := serverDef["command"].(string)
	var args []string
	if rawArgs, ok := serverDef["args"].([]any); ok {
		for _, a := range rawArgs {
			if s, ok := a.(string); ok {
				args = append(args, s)
			}
		}
	}

	cmd := exec.Command(command, args...)

	// Set environment variables if provided.
	if rawEnv, ok := serverDef["env"].(map[string]any); ok {
		for k, v := range rawEnv {
			if s, ok := v.(string); ok {
				cmd.Env = append(cmd.Env, k+"="+s)
			}
		}
	}

	return &sdkmcp.CommandTransport{Command: cmd}
}

func buildStreamableTransport(serverName string, serverDef map[string]any) *sdkmcp.StreamableClientTransport {
	url, _ := serverDef["url"].(string)
	return &sdkmcp.StreamableClientTransport{
		Endpoint:             url,
		HTTPClient:           buildHTTPClient(serverName, serverDef),
		DisableStandaloneSSE: true,
	}
}

func buildSSETransport(serverName string, serverDef map[string]any) *sdkmcp.SSEClientTransport {
	url, _ := serverDef["url"].(string)
	return &sdkmcp.SSEClientTransport{
		Endpoint:   url,
		HTTPClient: buildHTTPClient(serverName, serverDef),
	}
}

// tlsTransport is a shared base transport that skips TLS certificate
// verification. This is appropriate for a management CLI that connects to
// internal/development MCP servers which commonly use self-signed certificates
// or private CAs.
var tlsTransport = &http.Transport{
	TLSClientConfig: &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec // management tool for internal servers
	},
}

// buildHTTPClient creates an *http.Client that skips TLS verification,
// optionally injects auth headers from the server definition, and wraps
// with OAuth token injection when matching credentials exist.
func buildHTTPClient(serverName string, serverDef map[string]any) *http.Client {
	var base http.RoundTripper = tlsTransport

	// Wrap with header injection if headers are configured.
	if headers, ok := serverDef["headers"].(map[string]any); ok && len(headers) > 0 {
		h := make(http.Header, len(headers))
		for k, v := range headers {
			if s, ok := v.(string); ok {
				h.Set(k, s)
			}
		}
		base = &headerRoundTripper{base: base, headers: h}
	}

	// Wrap with OAuth if matching credentials exist and no static auth header
	// is already configured in the server definition.
	if !hasAuthHeader(serverDef) {
		serverURL, _ := serverDef["url"].(string)
		creds, err := config.LoadOAuthCredentials()
		if err == nil && len(creds) > 0 {
			entryKey := config.FindCredentialKey(creds, serverName, serverURL)
			if entryKey != "" {
				base = &oauthRoundTripper{
					base:     base,
					cred:     creds[entryKey],
					entryKey: entryKey,
				}
			}
		}
	}

	return &http.Client{Transport: base}
}

// hasAuthHeader returns true if the server definition already contains a static
// Authorization header, meaning OAuth injection should be skipped.
func hasAuthHeader(serverDef map[string]any) bool {
	headers, ok := serverDef["headers"].(map[string]any)
	if !ok {
		return false
	}
	for k := range headers {
		if strings.EqualFold(k, "Authorization") {
			return true
		}
	}
	return false
}

// headerRoundTripper injects static headers into every outgoing request.
type headerRoundTripper struct {
	base    http.RoundTripper
	headers http.Header
}

func (rt *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	for k, vals := range rt.headers {
		for _, v := range vals {
			req.Header.Set(k, v)
		}
	}
	return rt.base.RoundTrip(req)
}

// ExtractAutoApprove reads the autoApprove list from a server definition.
func ExtractAutoApprove(serverDef map[string]any) []string {
	raw, ok := serverDef["autoApprove"].([]any)
	if !ok {
		return nil
	}
	names := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			names = append(names, s)
		}
	}
	return names
}

// UpdateAutoApprove sets the autoApprove list on a server definition.
// When all discovered tools are selected, a wildcard ["*"] is stored instead
// of listing every tool name.
func UpdateAutoApprove(serverDef map[string]any, selected []string, totalTools int) {
	if len(selected) == 0 {
		delete(serverDef, "autoApprove")
		return
	}
	// Use wildcard when every discovered tool is selected.
	if len(selected) == totalTools {
		serverDef["autoApprove"] = []any{"*"}
		return
	}
	arr := make([]any, len(selected))
	for i, n := range selected {
		arr[i] = n
	}
	serverDef["autoApprove"] = arr
}

// IsAutoApproveWildcard returns true if the autoApprove list is ["*"].
func IsAutoApproveWildcard(approved []string) bool {
	return len(approved) == 1 && approved[0] == "*"
}

// FormatToolHint returns an emoji marker based on tool annotations.
// Returns empty string when the server didn't provide annotations.
func FormatToolHint(t ToolInfo) string {
	if !t.HasAnnotations {
		return ""
	}
	if t.ReadOnlyHint {
		return "👁️"
	}
	if t.DestructiveHint {
		return "⚠️"
	}
	return ""
}

