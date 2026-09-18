package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The rule the app has to keep: no unencrypted connection, whatever the URL
// says.
func TestSecureClientRefusesPlainHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("the request should never have reached the server")
	}))
	defer server.Close()

	_, err := secureHTTPClient(5 * time.Second).Get(server.URL)
	if err == nil {
		t.Fatal("expected an http:// request to be refused")
	}
	if !strings.Contains(err.Error(), "unencrypted") {
		t.Fatalf("the error should say why, got %q", err)
	}
}

// A server that redirects to http:// is the case a call-site check would miss.
func TestSecureClientRefusesADowngradeRedirect(t *testing.T) {
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("the redirect should never have been followed")
	}))
	defer plain.Close()

	secure := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL, http.StatusFound)
	}))
	defer secure.Close()

	// The TLS server's own certificate is self-signed, so the test client
	// trusts it while keeping the scheme rule under test.
	client := secureClient(secure.Client())
	if _, err := client.Get(secure.URL); err == nil {
		t.Fatal("expected the downgrade to be refused")
	}
}

// HTTPS itself still works, or the rule would have broken every call.
func TestSecureClientAllowsHTTPS(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	response, err := secureClient(server.Client()).Get(server.URL)
	if err != nil {
		t.Fatalf("an https request should succeed: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", response.StatusCode)
	}
}
