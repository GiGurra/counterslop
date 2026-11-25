// Package mcp is a small Model Context Protocol server: the Streamable HTTP
// transport (JSON-RPC 2.0 over POST, each response a JSON body; no server
// push), tools only, and the auth the game server needs (auth.go). The game's
// tools live in internal/game (mcp_tools.go); docs/mcp.md says how to use it.
//
// Hand-written rather than the official SDK: the game server needs four
// methods (initialize, ping, tools/list, tools/call) and keeps its
// dependency list short (go.mod).
package mcp

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
)

// Protocol versions this server speaks, newest first; a client asking for
// another gets the newest.
var ProtocolVersions = []string{"2025-06-18", "2025-03-26", "2024-11-05"}

// Tool is one MCP tool.
type Tool struct {
	Name        string
	Description string
	// Input is the JSON Schema of the arguments (an object schema); nil = no arguments.
	Input map[string]any
	// Call runs the tool. A returned error is a tool error the agent sees
	// (isError), not a protocol error. The result goes back as JSON text
	// (and as structuredContent when it is a JSON object).
	Call func(args json.RawMessage) (any, error)
}

// Text is a result returned as plain text, not JSON.
type Text string

// Server serves tools over MCP.
type Server struct {
	Name, Version string
	Instructions  string
	tools         []Tool
	// Log is told about every tool call (who: the auth that let it in); nil = slog.
	Log func(tool string, args json.RawMessage, err error)
}

// Add registers tools (in the order tools/list shows them).
func (s *Server) Add(t ...Tool) { s.tools = append(s.tools, t...) }

// Tools lists the registered tools' names.
func (s *Server) Tools() []string {
	out := make([]string, len(s.tools))
	for i, t := range s.tools {
		out[i] = t.Name
	}
	return out
}

type rpcReq struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResp struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcErr         `json:"error,omitempty"`
}

const maxBody = 1 << 20

// ServeHTTP is the MCP endpoint (already authenticated: Auth wraps it).
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
	case http.MethodGet: // no server-initiated stream: every answer comes back on its POST
		w.Header().Set("Allow", "POST")
		http.Error(w, "this MCP server sends no server-initiated messages; POST JSON-RPC", http.StatusMethodNotAllowed)
		return
	case http.MethodDelete: // no sessions to end
		w.WriteHeader(http.StatusNoContent)
		return
	default:
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBody+1))
	if err != nil || len(body) > maxBody {
		writeJSON(w, http.StatusBadRequest, rpcResp{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcErr{-32700, "request too large or unreadable"}})
		return
	}
	var req rpcReq
	if err := json.Unmarshal(body, &req); err != nil || req.JSONRPC != "2.0" || req.Method == "" {
		writeJSON(w, http.StatusBadRequest, rpcResp{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcErr{-32700, "not a JSON-RPC 2.0 request (batches are not supported)"}})
		return
	}
	if len(req.ID) == 0 || string(req.ID) == "null" { // a notification (or a response): accepted, no answer
		w.WriteHeader(http.StatusAccepted)
		return
	}
	res, rerr := s.dispatch(req)
	writeJSON(w, http.StatusOK, rpcResp{JSONRPC: "2.0", ID: req.ID, Result: res, Error: rerr})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) dispatch(req rpcReq) (any, *rpcErr) {
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		v := ProtocolVersions[0]
		if slices.Contains(ProtocolVersions, p.ProtocolVersion) {
			v = p.ProtocolVersion
		}
		res := map[string]any{
			"protocolVersion": v,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": s.Name, "version": s.Version},
		}
		if s.Instructions != "" {
			res["instructions"] = s.Instructions
		}
		return res, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		list := make([]map[string]any, 0, len(s.tools))
		for _, t := range s.tools {
			in := t.Input
			if in == nil {
				in = Object(nil)
			}
			list = append(list, map[string]any{"name": t.Name, "description": t.Description, "inputSchema": in})
		}
		return map[string]any{"tools": list}, nil
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, &rpcErr{-32602, "bad params"}
		}
		i := slices.IndexFunc(s.tools, func(t Tool) bool { return t.Name == p.Name })
		if i < 0 {
			return nil, &rpcErr{-32602, "unknown tool " + p.Name}
		}
		if len(p.Arguments) == 0 || string(p.Arguments) == "null" {
			p.Arguments = json.RawMessage("{}")
		}
		out, err := s.call(s.tools[i], p.Arguments)
		s.log(p.Name, p.Arguments, err)
		if err != nil {
			return map[string]any{"content": []any{map[string]any{"type": "text", "text": err.Error()}}, "isError": true}, nil
		}
		return toolResult(out), nil
	}
	return nil, &rpcErr{-32601, "method not found: " + req.Method}
}

// call runs a tool; a panic in it is the tool's error, not the server's end.
func (s *Server) call(t Tool, args json.RawMessage) (out any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("tool %s failed: %v", t.Name, r)
		}
	}()
	return t.Call(args)
}

func (s *Server) log(tool string, args json.RawMessage, err error) {
	if s.Log != nil {
		s.Log(tool, args, err)
		return
	}
	slog.Info("mcp tool", "tool", tool, "args", string(args), "err", err)
}

func toolResult(out any) map[string]any {
	if t, ok := out.(Text); ok {
		return map[string]any{"content": []any{map[string]any{"type": "text", "text": string(t)}}}
	}
	b, err := json.Marshal(out)
	if err != nil {
		return map[string]any{"content": []any{map[string]any{"type": "text", "text": "result not JSON: " + err.Error()}}, "isError": true}
	}
	res := map[string]any{"content": []any{map[string]any{"type": "text", "text": string(b)}}}
	if strings.HasPrefix(string(b), "{") {
		res["structuredContent"] = json.RawMessage(b)
	}
	return res
}

// Object is a JSON Schema object with these properties (name -> schema);
// required lists the required ones.
func Object(props map[string]any, required ...string) map[string]any {
	if props == nil {
		props = map[string]any{}
	}
	o := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		o["required"] = required
	}
	return o
}

// Str, Num, Int, Bool and Arr are property schemas with a description.
func Str(desc string) map[string]any  { return map[string]any{"type": "string", "description": desc} }
func Num(desc string) map[string]any  { return map[string]any{"type": "number", "description": desc} }
func Int(desc string) map[string]any  { return map[string]any{"type": "integer", "description": desc} }
func Bool(desc string) map[string]any { return map[string]any{"type": "boolean", "description": desc} }
func Arr(desc string, items map[string]any) map[string]any {
	return map[string]any{"type": "array", "description": desc, "items": items}
}

// Enum is a string property limited to values.
func Enum(desc string, values ...string) map[string]any {
	return map[string]any{"type": "string", "description": desc, "enum": values}
}

// Args decodes a tool's arguments into v, refusing unknown fields (a typo is
// an error the agent sees, not a silently ignored argument).
func Args(raw json.RawMessage, v any) error {
	d := json.NewDecoder(strings.NewReader(string(raw)))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return fmt.Errorf("arguments: %w", err)
	}
	return nil
}
