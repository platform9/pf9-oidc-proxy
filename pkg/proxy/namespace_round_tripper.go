// Copyright Platform9 Systems Inc See LICENSE for details.
package proxy

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// MappingManager manages the JSON mapping file

const (
	labelSelectorPrefix = "pcd-kaapi.pf9.io/region="
)

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
	defer func() {
		_ = file.Close()
	}()

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
	defer func() {
		_ = watcher.Close()
	}()

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
				if err := m.loadMappings(filePath); err != nil {
					fmt.Println("failed to reload mappings:", err)
				}
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
	c.addRegionLabelSelectorInPath(req)

	// Forward the request to the next RoundTripper
	return c.Transport.RoundTrip(req)
}

// modifyNamespaceInPath replaces the namespace in the URL path with the configured one
func (c *CustomNamespaceRoundTripper) modifyNamespaceInPath(path string) string {

	parts := strings.Split(path, "/")
	suffix := ""
	apiTokHit := false
	// if we reach "api" token we have gone too far we shouldn't continue looking for suffix
	for i, part := range parts {
		if part == "api" {
			apiTokHit = true
		}
		if suffix == "" && !apiTokHit {
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

func (c *CustomNamespaceRoundTripper) addRegionLabelSelectorInPath(req *http.Request) {
	partsOfPath := strings.Split(req.URL.Path, "/")

	// Get region name
	regionName := partsOfPath[1]

	// remove region name from request
	partsOfPath[1] = ""

	labelSelectorToBeAdded := []string{"byomachines", "byohosts", "hostedcontrolplanes", "openstackclusters", "machines", "machinedeployments", "clusters"}

	// Add region label selector to the path only if the request comes for labelSelectorToBeAdded objects

	/*
		/apis/{kind}/{version}/namespaces/{namespace}/{object}
	*/

	for i, partOfPath := range partsOfPath {
		if partOfPath == "namespaces" {
			/*
				if the request is for specific object with name, don't add label selector
				eg: /apis/{kind}/{version}/namespaces/{namespace}/{object}/{object-name}
				                              (i)       (i + 1)   (i + 2)   (i + 3)
			*/
			if len(partsOfPath) > i+3 && partsOfPath[i+3] != "" {
				// if the request is for all clusters in namespace, don't add label selector and remove all-clusters-in-pf9-tenant-namespace from path
				if partsOfPath[i+2] == "clusters" && partsOfPath[i+3] == "all-clusters-in-pf9-tenant-namespace" {
					partsOfPath[i+3] = ""
				}
				break
			}

			// check if object is in labelSelectorToBeAdded
			if slices.Contains(labelSelectorToBeAdded, partsOfPath[i+2]) {
				// add region label selector
				query := req.URL.Query()

				selector := labelSelectorPrefix + regionName
				query.Set("labelSelector", selector)
				req.URL.RawQuery = query.Encode()
			}
		}
	}
	req.URL.Path = joinPath(partsOfPath)

}

func joinPath(parts []string) string {
	var filteredPrarts []string
	for _, part := range parts {
		if part != "" {
			filteredPrarts = append(filteredPrarts, part)
		}
	}
	fullPath := strings.Join(filteredPrarts, "/")
	if !strings.HasPrefix(fullPath, "/") {
		fullPath = "/" + fullPath
	}
	return fullPath
}
