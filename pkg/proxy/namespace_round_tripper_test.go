package proxy

import (
	"testing"
)

func TestModifyNamespaceInPath(t *testing.T) {
	// Create a temporary mapping file
	mappings := map[string]string{
		"tenant1": "namespace1",
		"tenant2": "namespace2",
	}

	// Create a mapping manager with the test mappings
	manager := &MappingManager{
		mappings: mappings,
	}

	// Create a CustomNamespaceRoundTripper with the test mapping manager
	tripper := &CustomNamespaceRoundTripper{
		MappingManager:   manager,
		DefaultNamespace: "default",
	}

	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "basic namespace replacement",
			path:     "/tenant1/api/v1/namespaces/default/pods",
			expected: "/api/v1/namespaces/namespace1/pods",
		},
		{
			name:     "path without namespace",
			path:     "/api/v1/pods",
			expected: "/api/v1/pods",
		},
		{
			name:     "path with unmapped tenant",
			path:     "/unknown-tenant/api/v1/namespaces/default/pods",
			expected: "/api/v1/namespaces/default/pods",
		},
		{
			name:     "path with tenant but no namespace",
			path:     "/tenant1/api?timeout=50s",
			expected: "/api?timeout=50s",
		},
		{
			name:     "path with tenant and a request to list namespaces",
			path:     "/tenant1/api/v1/namespaces?limit=500",
			expected: "/api/v1/namespaces?limit=500",
		},
		{
			name:     "path with multiple namespaces segments",
			path:     "/tenant2/api/v1/namespaces/default/pods/namespaces",
			expected: "/api/v1/namespaces/namespace2/pods/namespaces",
		},
		{
			name:     "path with api before tenant, no changes expected",
			path:     "/api/tenant1/v1/namespaces/default/pods",
			expected: "/api/tenant1/v1/namespaces/default/pods",
		},
		{
			name:     "empty path",
			path:     "",
			expected: "/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tripper.modifyNamespaceInPath(tt.path)
			if result != tt.expected {
				t.Errorf("modifyNamespaceInPath(%q) = %q, want %q",
					tt.path, result, tt.expected)
			}
		})
	}
}
