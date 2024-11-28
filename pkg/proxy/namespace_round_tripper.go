// Copyright Platform9 Systems Inc See LICENSE for details.
package proxy

import (
	"net/http"
	"strings"
)

// CustomNamespaceRoundTripper modifies the namespace in Kubernetes API requests
type CustomNamespaceRoundTripper struct {
	Transport          http.RoundTripper
	// the namespace that overrides the namespace in the URL
	NamespaceOverride string
}

// RoundTrip implements the RoundTripper interface that takes in the req for k8s API
// and replaces the namespace in the URL with NamespaceOverride
func (c *CustomNamespaceRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Modify the namespace in the URL path
	req.URL.Path = c.modifyNamespaceInPath(req.URL.Path)

	// Forward the request to the next RoundTripper
	return c.Transport.RoundTrip(req)
}

// modifyNamespaceInPath replaces the namespace in the URL path with the configured one
func (c *CustomNamespaceRoundTripper) modifyNamespaceInPath(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if part == "namespaces" && i+1 < len(parts) {
			// Replace the namespace part
			parts[i+1] = c.NamespaceOverride
			break
		}
	}
	return strings.Join(parts, "/")
}