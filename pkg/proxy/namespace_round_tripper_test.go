// Copyright Platform9 Systems Inc See LICENSE for details.

package proxy

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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
		{
			name:     "tenant segment is 'api', should not modify",
			path:     "/api/api/v1/namespaces/default/pods",
			expected: "/api/api/v1/namespaces/default/pods",
		},
		{
			name:     "malformed path with no leading slash",
			path:     "tenant1/api/v1/namespaces/default/pods",
			expected: "/api/v1/namespaces/namespace1/pods",
		},
		{
			name:     "tenant exists but no 'namespaces' keyword",
			path:     "/tenant1/api/v1/pods",
			expected: "/api/v1/pods",
		},
		{
			name:     "tenant not mapped, preserve original namespace",
			path:     "/tenantX/api/v1/namespaces/kube-system/pods",
			expected: "/api/v1/namespaces/kube-system/pods",
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

// writeProjectedVersion lays down one timestamped payload directory the way
// kubelet's atomic writer does, and returns its path.
func writeProjectedVersion(t *testing.T, dir, stamp, content string) string {
	t.Helper()
	d := filepath.Join(dir, stamp)
	if err := os.Mkdir(d, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", d, err)
	}
	if err := os.WriteFile(filepath.Join(d, "ns-mapping.json"), []byte(content), 0o644); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	return d
}

// TestMappingManagerReloadsOnConfigMapSwap reproduces how kubelet updates a
// projected ConfigMap volume:
//
//	ns-mapping.json -> ..data/ns-mapping.json
//	..data          -> ..<timestamp>
//
// An update writes a new timestamped directory, creates ..data_tmp, renames it
// onto ..data, then deletes the old directory. The mapping file is never
// written in place and its target inode is replaced, so a watcher registered
// on the file itself goes deaf after the first update.
func TestMappingManagerReloadsOnConfigMapSwap(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "ns-mapping.json")

	first := writeProjectedVersion(t, dir, "..2000_01_01_00_00_00", `{"tenant1":"ns-one"}`)
	if err := os.Symlink(filepath.Base(first), filepath.Join(dir, "..data")); err != nil {
		t.Fatalf("symlink ..data: %v", err)
	}
	if err := os.Symlink(filepath.Join("..data", "ns-mapping.json"), file); err != nil {
		t.Fatalf("symlink mapping file: %v", err)
	}

	m, err := NewMappingManager(file)
	if err != nil {
		t.Fatalf("NewMappingManager: %v", err)
	}
	if ns, ok := m.GetNamespace("tenant1"); !ok || ns != "ns-one" {
		t.Fatalf("initial load: got (%q, %v), want (%q, true)", ns, ok, "ns-one")
	}

	// kubelet's atomic swap.
	second := writeProjectedVersion(t, dir, "..2000_01_01_00_00_01", `{"tenant1":"ns-two"}`)
	tmp := filepath.Join(dir, "..data_tmp")
	if err := os.Symlink(filepath.Base(second), tmp); err != nil {
		t.Fatalf("symlink ..data_tmp: %v", err)
	}
	if err := os.Rename(tmp, filepath.Join(dir, "..data")); err != nil {
		t.Fatalf("rename onto ..data: %v", err)
	}
	if err := os.RemoveAll(first); err != nil {
		t.Fatalf("remove old payload: %v", err)
	}

	// Reloading is asynchronous.
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		if ns, ok := m.GetNamespace("tenant1"); ok && ns == "ns-two" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	ns, _ := m.GetNamespace("tenant1")
	t.Fatalf("mapping not reloaded after ConfigMap swap: got %q, want %q", ns, "ns-two")
}
