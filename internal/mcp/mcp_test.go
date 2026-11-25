package mcp

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func testServer() *Server {
	s := &Server{Name: "t", Version: "1", Instructions: "hi"}
	s.Log = func(string, json.RawMessage, error) {}
	s.Add(Tool{Name: "echo", Description: "echo", Input: Object(map[string]any{"x": Int("x")}, "x"),
		Call: func(raw json.RawMessage) (any, error) {
			var a struct{ X int }
			if err := Args(raw, &a); err != nil {
				return nil, err
			}
			return map[string]int{"x": a.X}, nil
		}},
		Tool{Name: "text", Call: func(json.RawMessage) (any, error) { return Text("plain"), nil }},
		Tool{Name: "fail", Call: func(json.RawMessage) (any, error) { return nil, errors.New("nope") }},
		Tool{Name: "panic", Call: func(json.RawMessage) (any, error) { panic("boom") }})
	return s
}

func post(t *testing.T, h http.Handler, path, body string, hdr map[string]string) (int, map[string]any, http.Header) {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	var m map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	return w.Code, m, w.Header()
}

func rpc(method string, params string) string {
	if params == "" {
		params = "{}"
	}
	return `{"jsonrpc":"2.0","id":7,"method":"` + method + `","params":` + params + `}`
}

func TestProtocol(t *testing.T) {
	s := testServer()
	code, m, _ := post(t, s, "/", rpc("initialize", `{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"c","version":"0"}}`), nil)
	res := m["result"].(map[string]any)
	if code != 200 || m["id"].(float64) != 7 || res["protocolVersion"] != "2025-03-26" || res["serverInfo"].(map[string]any)["name"] != "t" || res["instructions"] != "hi" {
		t.Fatalf("initialize: %d %v", code, m)
	}
	if _, m, _ = post(t, s, "/", rpc("initialize", `{"protocolVersion":"1999-01-01"}`), nil); m["result"].(map[string]any)["protocolVersion"] != ProtocolVersions[0] {
		t.Fatalf("unknown version: %v", m)
	}
	if code, _, _ := post(t, s, "/", `{"jsonrpc":"2.0","method":"notifications/initialized"}`, nil); code != http.StatusAccepted {
		t.Fatalf("notification: %d", code)
	}
	if _, m, _ = post(t, s, "/", rpc("ping", ""), nil); m["result"] == nil {
		t.Fatalf("ping %v", m)
	}
	_, m, _ = post(t, s, "/", rpc("tools/list", ""), nil)
	tools := m["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 4 || tools[0].(map[string]any)["name"] != "echo" || tools[1].(map[string]any)["inputSchema"].(map[string]any)["type"] != "object" {
		t.Fatalf("tools/list %v", tools)
	}
	call := func(name, args string) map[string]any {
		_, m, _ := post(t, s, "/", rpc("tools/call", `{"name":"`+name+`","arguments":`+args+`}`), nil)
		return m
	}
	r := call("echo", `{"x":3}`)["result"].(map[string]any)
	if r["structuredContent"].(map[string]any)["x"].(float64) != 3 || r["content"].([]any)[0].(map[string]any)["text"] != `{"x":3}` || r["isError"] != nil {
		t.Fatalf("echo %v", r)
	}
	if r := call("text", `{}`)["result"].(map[string]any); r["content"].([]any)[0].(map[string]any)["text"] != "plain" || r["structuredContent"] != nil {
		t.Fatalf("text %v", r)
	}
	for _, c := range []struct{ name, args, want string }{{"echo", `{"y":1}`, "unknown field"}, {"fail", `{}`, "nope"}, {"panic", `{}`, "boom"}} {
		r := call(c.name, c.args)["result"].(map[string]any)
		if r["isError"] != true || !strings.Contains(r["content"].([]any)[0].(map[string]any)["text"].(string), c.want) {
			t.Fatalf("%s: %v", c.name, r)
		}
	}
	if e := call("nope", `{}`)["error"].(map[string]any); e["code"].(float64) != -32602 {
		t.Fatalf("unknown tool %v", e)
	}
	if _, m, _ = post(t, s, "/", rpc("resources/list", ""), nil); m["error"].(map[string]any)["code"].(float64) != -32601 {
		t.Fatalf("unknown method %v", m)
	}
	if code, _, _ := post(t, s, "/", `[{"jsonrpc":"2.0","id":1,"method":"ping"}]`, nil); code != 400 {
		t.Fatalf("batch: %d", code)
	}
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		w := httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest(method, "/", nil))
		if w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s: %d", method, w.Code)
		}
	}
}

func TestAuthCheck(t *testing.T) {
	long := "0123456789abcdef"
	for _, c := range []struct {
		a    Auth
		addr string
		ok   bool
	}{
		{Auth{}, "127.0.0.1:1", false}, // no auth at all: refused, even on localhost
		{Auth{}, "0.0.0.0:1", false},
		{Auth{Token: long}, "127.0.0.1:1", true},
		{Auth{Token: "short"}, "127.0.0.1:1", false},
		{Auth{PathSecret: long}, "0.0.0.0:1", true},
		{Auth{PathSecret: long + "/x"}, "0.0.0.0:1", false},
		{Auth{ClientID: "c", ClientSecret: long}, "0.0.0.0:1", true},
		{Auth{ClientID: "c"}, "0.0.0.0:1", false},
		{Auth{Token: long}, "nonsense", false},
	} {
		if err := c.a.Check(c.addr); (err == nil) != c.ok {
			t.Errorf("%+v on %s: %v", c.a, c.addr, err)
		}
	}
	if ListenAddr(":8091") != "127.0.0.1:8091" || ListenAddr("8091") != "127.0.0.1:8091" || ListenAddr("0.0.0.0:8091") != "0.0.0.0:8091" {
		t.Fatal("ListenAddr")
	}
	if !Loopback("127.0.0.1:1") || !Loopback("localhost:1") || !Loopback("[::1]:1") || Loopback("0.0.0.0:1") || Loopback("10.0.0.1:1") {
		t.Fatal("Loopback")
	}
}

const ping = `{"jsonrpc":"2.0","id":1,"method":"ping"}`

func TestAuthToken(t *testing.T) {
	h := Auth{Token: "0123456789abcdef"}.Handler(testServer())
	if code, _, hdr := post(t, h, "/mcp", ping, nil); code != 401 || !strings.Contains(hdr.Get("WWW-Authenticate"), "resource_metadata") {
		t.Fatalf("no token: %d %v", code, hdr)
	}
	if code, _, _ := post(t, h, "/mcp", ping, map[string]string{"Authorization": "Bearer wrong"}); code != 401 {
		t.Fatalf("wrong token: %d", code)
	}
	if code, m, _ := post(t, h, "/mcp", ping, map[string]string{"Authorization": "Bearer 0123456789abcdef"}); code != 200 || m["result"] == nil {
		t.Fatalf("token: %d %v", code, m)
	}
	if code, _, _ := post(t, h, "/mcp/0123456789abcdef", ping, nil); code != 404 {
		t.Fatalf("a path without a path secret configured: %d", code)
	}
	if code, _, _ := post(t, h, "/oauth/token", "grant_type=client_credentials", nil); code != 404 {
		t.Fatalf("oauth without client credentials configured: %d", code)
	}
}

func TestAuthPathSecret(t *testing.T) {
	h := Auth{PathSecret: "s3cr3t-s3cr3t-s3cr3t"}.Handler(testServer())
	if code, m, _ := post(t, h, "/mcp/s3cr3t-s3cr3t-s3cr3t", ping, nil); code != 200 || m["result"] == nil {
		t.Fatalf("secret path: %d %v", code, m)
	}
	if code, _, _ := post(t, h, "/mcp/s3cr3t-s3cr3t-s3cr3T", ping, nil); code != 404 {
		t.Fatalf("wrong secret: %d", code)
	}
	if code, _, _ := post(t, h, "/mcp", ping, nil); code != 401 {
		t.Fatalf("plain /mcp: %d", code)
	}
}

func TestAuthOAuthClientCredentials(t *testing.T) {
	now := time.Unix(1000, 0)
	a := Auth{ClientID: "agent", ClientSecret: "0123456789abcdef", TokenTTL: time.Minute, Now: func() time.Time { return now }}
	h := a.Handler(testServer())
	srv := httptest.NewServer(h)
	defer srv.Close()
	get := func(path string) map[string]any {
		res, err := http.Get(srv.URL + path)
		if err != nil || res.StatusCode != 200 {
			t.Fatalf("%s: %v %v", path, err, res)
		}
		var m map[string]any
		_ = json.NewDecoder(res.Body).Decode(&m)
		res.Body.Close()
		return m
	}
	pr := get("/.well-known/oauth-protected-resource")
	as := get("/.well-known/oauth-authorization-server")
	if pr["resource"] != srv.URL+"/mcp" || pr["authorization_servers"].([]any)[0] != srv.URL || as["token_endpoint"] != srv.URL+"/oauth/token" {
		t.Fatalf("metadata %v %v", pr, as)
	}
	token := func(form url.Values, basic bool) (int, map[string]any) {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/oauth/token", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if basic {
			req.SetBasicAuth("agent", "0123456789abcdef")
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var m map[string]any
		b, _ := io.ReadAll(res.Body)
		_ = json.Unmarshal(b, &m)
		return res.StatusCode, m
	}
	if code, m := token(url.Values{"grant_type": {"client_credentials"}, "client_id": {"agent"}, "client_secret": {"wrong-wrong-wrong"}}, false); code != 401 || m["error"] != "invalid_client" {
		t.Fatalf("wrong secret: %d %v", code, m)
	}
	if code, m := token(url.Values{"grant_type": {"password"}}, true); code != 400 || m["error"] != "unsupported_grant_type" {
		t.Fatalf("grant type: %d %v", code, m)
	}
	code, m := token(url.Values{"grant_type": {"client_credentials"}}, true)
	if code != 200 || m["token_type"] != "Bearer" || m["expires_in"].(float64) != 60 {
		t.Fatalf("basic: %d %v", code, m)
	}
	tok := m["access_token"].(string)
	if code, m := token(url.Values{"grant_type": {"client_credentials"}, "client_id": {"agent"}, "client_secret": {"0123456789abcdef"}}, false); code != 200 || m["access_token"] == tok {
		t.Fatalf("form: %d %v", code, m)
	}
	auth := map[string]string{"Authorization": "Bearer " + tok}
	if code, _, _ := post(t, h, "/mcp", ping, auth); code != 200 {
		t.Fatalf("issued token: %d", code)
	}
	now = now.Add(61 * time.Second)
	if code, _, _ := post(t, h, "/mcp", ping, auth); code != 401 {
		t.Fatalf("expired token: %d", code)
	}
}
