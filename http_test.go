package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestStartHTTPServerReturnsListenError(t *testing.T) {
	t.Parallel()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve address: %v", err)
	}
	defer listener.Close()

	config := &Config{
		McpProxy: &MCPProxyConfigV2{
			BaseURL: "http://" + listener.Addr().String(),
			Addr:    listener.Addr().String(),
			Name:    "test",
			Version: "test",
			Type:    MCPServerTypeStreamable,
			Options: &OptionsV2{},
		},
		McpServers: map[string]*MCPClientConfigV2{},
	}

	err = startHTTPServer(config)
	if err == nil || !strings.Contains(err.Error(), "HTTP server failed") {
		t.Fatalf("startHTTPServer error = %v, want listen failure", err)
	}
}

// A server that fails to connect still gets its route mounted, responding
// with a clear 503 instead of a bare 404 - see docs/OAUTH_LIFECYCLE.md.
func TestConnectAndMountMountsNotReadyRouteOnFailure(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer upstream.Close()

	clientConfig := &MCPClientConfigV2{
		TransportType: MCPClientTypeStreamable,
		URL:           upstream.URL,
		Options:       &OptionsV2{},
	}
	mcpClient, err := newMCPClient("broken", clientConfig)
	if err != nil {
		t.Fatalf("newMCPClient: %v", err)
	}
	defer mcpClient.Close()

	srv, err := newMCPServer("broken", &MCPProxyConfigV2{
		Type:    MCPServerTypeStreamable,
		Version: "test",
	}, clientConfig)
	if err != nil {
		t.Fatalf("newMCPServer: %v", err)
	}

	baseURL, err := url.Parse("http://example.com")
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	httpMux := http.NewServeMux()

	connErr := connectAndMount(t.Context(), "broken", clientConfig, mcp.Implementation{Name: "test"}, mcpClient, srv, baseURL, httpMux)
	if connErr == nil {
		t.Fatal("connectAndMount error = nil, want error from failed connect")
	}

	req := httptest.NewRequest(http.MethodGet, "/broken/", nil)
	rec := httptest.NewRecorder()
	httpMux.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if !strings.Contains(rec.Body.String(), "broken") {
		t.Fatalf("body = %q, want it to mention the server name", rec.Body.String())
	}
}
