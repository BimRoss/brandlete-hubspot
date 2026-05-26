package mcp

import (
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/BimRoss/brandlete-hubspot/internal/hubspot"
)

// Minimal MCP-over-HTTP (JSON-RPC 2.0) server for v0.
// Implements: initialize, tools/list, tools/call. Single request/response;
// no SSE streaming yet — adequate for Claude Desktop / Cursor over HTTP.
// TODO: upgrade to full Streamable HTTP transport when we need server-initiated messages.

const (
	protocolVersion = "2025-06-18"
	serverName      = "brandlete-hubspot-mcp"
	serverVersion   = "0.0.1"
)

func Register(mux *http.ServeMux, hs *hubspot.Client, bearer string, log *slog.Logger) {
	s := &server{hs: hs, bearer: bearer, log: log}
	mux.HandleFunc("POST /mcp", s.authMiddleware(s.handle))
}

type server struct {
	hs     *hubspot.Client
	bearer string
	log    *slog.Logger
}

func (s *server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		want := "Bearer " + s.bearer
		if !strings.HasPrefix(auth, "Bearer ") || subtle.ConstantTimeCompare([]byte(auth), []byte(want)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (s *server) handle(w http.ResponseWriter, r *http.Request) {
	var req rpcRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeRPC(w, rpcResponse{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: "parse error"}})
		return
	}

	// Notifications (no id) — accept and return empty.
	if len(req.ID) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	resp := rpcResponse{JSONRPC: "2.0", ID: req.ID}

	switch req.Method {
	case "initialize":
		resp.Result = map[string]any{
			"protocolVersion": protocolVersion,
			"serverInfo":      map[string]string{"name": serverName, "version": serverVersion},
			"capabilities":    map[string]any{"tools": map[string]any{}},
		}
	case "tools/list":
		resp.Result = map[string]any{"tools": toolDefs()}
	case "tools/call":
		var p struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		_ = json.Unmarshal(req.Params, &p)
		result, err := s.callTool(r, p.Name, p.Arguments)
		if err != nil {
			resp.Result = map[string]any{
				"isError": true,
				"content": []map[string]any{{"type": "text", "text": err.Error()}},
			}
		} else {
			resp.Result = result
		}
	default:
		resp.Error = &rpcError{Code: -32601, Message: "method not found: " + req.Method}
	}

	writeRPC(w, resp)
}

func writeRPC(w http.ResponseWriter, resp rpcResponse) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func toolDefs() []map[string]any {
	return []map[string]any{
		{
			"name":        "whoami",
			"description": "Returns the HubSpot account this MCP server is authed against. Smoke test.",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			"name":        "search-contacts",
			"description": "Search HubSpot contacts by name, email, or company. Returns matching contacts.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string", "description": "Search query"},
				},
				"required": []string{"query"},
			},
		},
		// TODO: search-deals, get-contact-activity, add-note
	}
}

func (s *server) callTool(r *http.Request, name string, args map[string]any) (any, error) {
	switch name {
	case "whoami":
		info, err := s.hs.WhoAmI(r.Context())
		if err != nil {
			return nil, err
		}
		return textResult(info), nil
	case "search-contacts":
		q, _ := args["query"].(string)
		contacts, err := s.hs.SearchContacts(r.Context(), q)
		if err != nil {
			return nil, err
		}
		return textResult(contacts), nil
	default:
		return nil, &toolError{msg: "unknown tool: " + name}
	}
}

type toolError struct{ msg string }

func (e *toolError) Error() string { return e.msg }

func textResult(v any) map[string]any {
	b, _ := json.MarshalIndent(v, "", "  ")
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": string(b)}},
	}
}
