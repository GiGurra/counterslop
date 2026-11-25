package mcp

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Auth configures who may use the MCP endpoint. At least one way in is
// required (Check): a server never runs an open admin interface.
//
//	Token         a static bearer token (Authorization: Bearer <token>)
//	PathSecret    the endpoint is /mcp/<secret> (for clients that can't send headers)
//	ClientID/Secret  OAuth 2 client credentials: POST /oauth/token issues
//	              short-lived bearer tokens (RFC 6749 4.4), advertised per the
//	              MCP auth spec (RFC 9728 protected resource metadata, RFC 8414
//	              authorization server metadata)
//
// Secrets come from the environment (main.go: boa env vars) and are never
// logged.
type Auth struct {
	Token                  string
	PathSecret             string
	ClientID, ClientSecret string
	TokenTTL               time.Duration    // OAuth tokens (default 1 h)
	Now                    func() time.Time // the clock for token expiry (default time.Now)
}

// minSecret: secrets shorter than this are refused (guessable).
const minSecret = 16

// Check validates the configuration for listening on addr.
func (a Auth) Check(addr string) error {
	if a.Token == "" && a.PathSecret == "" && a.ClientID == "" {
		return errors.New("the MCP needs auth: --mcp-token, --mcp-path-secret or --mcp-client-id + --mcp-client-secret (env COUNTERSLOP_MCP_*)")
	}
	for name, v := range map[string]string{"token": a.Token, "path secret": a.PathSecret, "client secret": a.ClientSecret} {
		if v != "" && len(v) < minSecret {
			return fmt.Errorf("the MCP %s is too short (at least %d characters)", name, minSecret)
		}
	}
	if a.PathSecret != "" && strings.ContainsAny(a.PathSecret, "/?#% ") {
		return errors.New("the MCP path secret must be one URL path segment (no / ? # % or spaces)")
	}
	if (a.ClientID == "") != (a.ClientSecret == "") {
		return errors.New("OAuth needs both --mcp-client-id and --mcp-client-secret")
	}
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return fmt.Errorf("--mcp-addr %q: %w", addr, err)
	}
	return nil
}

// ListenAddr is where to listen for addr: a port alone (":8091" or "8091")
// binds the loopback interface; to expose the MCP, name a host ("0.0.0.0:8091").
func ListenAddr(addr string) string {
	if !strings.Contains(addr, ":") {
		addr = ":" + addr
	}
	if strings.HasPrefix(addr, ":") {
		return "127.0.0.1" + addr
	}
	return addr
}

// Loopback: addr only listens on this machine.
func Loopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Handler serves the MCP endpoint behind auth, plus the OAuth endpoints
// when client credentials are configured:
//
//	/mcp or /mcp/<secret>                      the MCP endpoint
//	/oauth/token                               client credentials -> bearer token
//	/.well-known/oauth-protected-resource      RFC 9728
//	/.well-known/oauth-authorization-server    RFC 8414
//
// who tells the logger which way a request came in ("token", "path", "oauth").
func (a Auth) Handler(mcp http.Handler) http.Handler {
	if a.TokenTTL <= 0 {
		a.TokenTTL = time.Hour
	}
	if a.Now == nil {
		a.Now = time.Now
	}
	tok := &tokens{m: map[string]time.Time{}}
	mux := http.NewServeMux()
	guarded := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.bearerOK(r, tok) {
			mcp.ServeHTTP(w, r)
			return
		}
		w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer resource_metadata="%s/.well-known/oauth-protected-resource"`, baseURL(r)))
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
	mux.Handle("/mcp", guarded)
	if a.PathSecret != "" {
		mux.Handle("/mcp/{secret}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if subtle.ConstantTimeCompare([]byte(r.PathValue("secret")), []byte(a.PathSecret)) == 1 {
				mcp.ServeHTTP(w, r)
				return
			}
			http.NotFound(w, r) // a wrong secret looks like nothing there
		}))
	}
	if a.ClientID != "" {
		mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) { a.token(w, r, tok) })
		mux.HandleFunc("/.well-known/oauth-protected-resource", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusOK, map[string]any{"resource": baseURL(r) + "/mcp", "authorization_servers": []string{baseURL(r)},
				"bearer_methods_supported": []string{"header"}})
		})
		mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusOK, map[string]any{"issuer": baseURL(r), "token_endpoint": baseURL(r) + "/oauth/token",
				"grant_types_supported": []string{"client_credentials"}, "token_endpoint_auth_methods_supported": []string{"client_secret_basic", "client_secret_post"},
				"response_types_supported": []string{}})
		})
	}
	return mux
}

func baseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func (a Auth) bearerOK(r *http.Request, tok *tokens) bool {
	h := r.Header.Get("Authorization")
	t, ok := strings.CutPrefix(h, "Bearer ")
	if !ok || t == "" {
		return false
	}
	if a.Token != "" && subtle.ConstantTimeCompare([]byte(t), []byte(a.Token)) == 1 {
		return true
	}
	return a.ClientID != "" && tok.valid(t, a.Now())
}

// tokens are the OAuth bearer tokens issued, with their expiry.
type tokens struct {
	mu sync.Mutex
	m  map[string]time.Time
}

func (t *tokens) issue(now time.Time, ttl time.Duration) string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	s := hex.EncodeToString(b)
	t.mu.Lock()
	defer t.mu.Unlock()
	for k, exp := range t.m { // drop the expired ones
		if !now.Before(exp) {
			delete(t.m, k)
		}
	}
	t.m[s] = now.Add(ttl)
	return s
}

func (t *tokens) valid(s string, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	exp, ok := t.m[s]
	return ok && now.Before(exp)
}

// token is the OAuth token endpoint: grant_type=client_credentials, the
// client authenticated by HTTP Basic or client_id/client_secret in the form.
func (a Auth) token(w http.ResponseWriter, r *http.Request, tok *tokens) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "invalid_request"})
		return
	}
	if err := r.ParseForm(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	if r.PostForm.Get("grant_type") != "client_credentials" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported_grant_type"})
		return
	}
	id, secret, basic := r.BasicAuth()
	if !basic {
		id, secret = r.PostForm.Get("client_id"), r.PostForm.Get("client_secret")
	}
	if subtle.ConstantTimeCompare([]byte(id), []byte(a.ClientID)) != 1 || subtle.ConstantTimeCompare([]byte(secret), []byte(a.ClientSecret)) != 1 {
		if basic {
			w.Header().Set("WWW-Authenticate", `Basic realm="counterslop-mcp"`)
		}
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_client"})
		return
	}
	t := tok.issue(a.Now(), a.TokenTTL)
	b, _ := json.Marshal(map[string]any{"access_token": t, "token_type": "Bearer", "expires_in": int(a.TokenTTL.Seconds())})
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
}
