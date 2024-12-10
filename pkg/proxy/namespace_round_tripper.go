// Copyright Platform9 Systems Inc See LICENSE for details.
package proxy

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// MappingManager manages the JSON mapping file
type MappingManager struct {
	mappings map[string]string
	mutex    sync.RWMutex
}

// NewMappingManager creates a new MappingManager
func NewMappingManager(filePath string) (*MappingManager, error) {
	manager := &MappingManager{
		mappings: make(map[string]string),
	}
	// Load initial mappings
	if err := manager.loadMappings(filePath); err != nil {
		return nil, err
	}
	// Watch for file changes
	go manager.watchFile(filePath)
	return manager, nil
}

// loadMappings loads mappings from the JSON file
func (m *MappingManager) loadMappings(filePath string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	data, err := ioutil.ReadAll(file)
	if err != nil {
		return err
	}

	newMappings := make(map[string]string)
	if err := json.Unmarshal(data, &newMappings); err != nil {
		return err
	}
	m.mappings = newMappings
	return nil
}

// watchFile monitors the file for changes and reloads it
func (m *MappingManager) watchFile(filePath string) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Println("Failed to create file watcher:", err)
		return
	}
	defer watcher.Close()

	err = watcher.Add(filePath)
	if err != nil {
		fmt.Println("Failed to watch file:", err)
		return
	}

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Write == fsnotify.Write {
				fmt.Println("Mapping file updated, reloading...")
				m.loadMappings(filePath)
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			fmt.Println("File watcher error:", err)
		}
	}
}

// GetNamespace retrieves the namespace for a given path suffix
func (m *MappingManager) GetNamespace(pathSuffix string) (string, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	namespace, exists := m.mappings[pathSuffix]
	return namespace, exists
}

func NewCustomNamespaceRoundTripper(suffixNSMappingFile string) (*CustomNamespaceRoundTripper, error) {

	mappingManager, err := NewMappingManager(suffixNSMappingFile)
	if err != nil {
		return nil, fmt.Errorf("error initializing mapping manager: %v", err)
	}

	return &CustomNamespaceRoundTripper{
		Transport:           http.DefaultTransport,
		MappingManager:      mappingManager,
		SuffixNSMappingFile: suffixNSMappingFile,
		DefaultNamespace:    "default",
	}, nil
}

// CustomNamespaceRoundTripper modifies the namespace in Kubernetes API requests
type CustomNamespaceRoundTripper struct {
	Transport http.RoundTripper
	// the namespace that overrides the namespace in the URL
	SuffixNSMappingFile string
	MappingManager      *MappingManager
	DefaultNamespace    string
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
// it removes the tenant prefix and replaces the namespace with the configured one
func (c *CustomNamespaceRoundTripper) modifyNamespaceInPath(path string) string {
	parts := strings.Split(path, "/")
	suffix := ""
	apiTokHit := false
	// if we reach "api" token we have gone too far we shouldn't continue looking for suffix
	for i, part := range parts {
		if part == "api" {
			apiTokHit = true
		}
		if suffix == "" && apiTokHit == false {
			suffix = part
			parts[i] = ""
		}
		if part == "namespaces" && i+1 < len(parts) {
			if namespace, exists := c.MappingManager.GetNamespace(suffix); exists {
				parts[i+1] = namespace
			}
			break
		}
	}
	return joinPath(parts)
}

func joinPath(parts []string) string {
	var filteredPrarts []string
	for _, part := range parts {
		if part != "" {
			filteredPrarts = append(filteredPrarts, part)
		}
	}
	fullPath := strings.Join(filteredPrarts, "/")
	if false == strings.HasPrefix(fullPath, "/") {
		fullPath = "/" + fullPath
	}
	return fullPath
}
