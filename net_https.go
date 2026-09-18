package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Every service this app talks to answers on HTTPS: Getty's SPARQL endpoint,
// Google's OAuth and upload endpoints, and the update check. Nothing it does
// needs an unencrypted connection, so nothing it does is allowed one.
//
// The rule is enforced at the transport rather than at each call site, because
// a call site can be added later and a redirect is not a call site at all: a
// server that answers with a 302 to an http:// URL would otherwise downgrade
// the connection with no code here mentioning HTTP.

// httpsOnly refuses any request that is not HTTPS.
type httpsOnly struct{ base http.RoundTripper }

func (t httpsOnly) RoundTrip(request *http.Request) (*http.Response, error) {
	if !strings.EqualFold(request.URL.Scheme, "https") {
		return nil, fmt.Errorf("refusing an unencrypted connection to %s: %s:// is not allowed",
			request.URL.Host, request.URL.Scheme)
	}
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(request)
}

// secureHTTPClient returns a client that will only talk HTTPS.
func secureHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: httpsOnly{}}
}

// secureClient applies the same rule to a client built elsewhere, such as the
// one the OAuth library hands back.
func secureClient(client *http.Client) *http.Client {
	if client == nil {
		return secureHTTPClient(0)
	}
	secured := *client
	secured.Transport = httpsOnly{base: client.Transport}
	return &secured
}
