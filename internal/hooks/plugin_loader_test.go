package hooks

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestPluginLoader_LoadPlugin(t *testing.T) {
	loader := NewPluginLoader(slog.Default())

	// Test loading a non-existent plugin
	_, err := loader.LoadPlugin("nonexistent", "/nonexistent/path.so", nil)
	if err == nil {
		t.Error("Expected error when loading non-existent plugin")
	}

	// Test with invalid plugin file (if we have one)
	// This would require a valid .so file for a full test
}

func TestPluginLoader_DiscoverPlugins(t *testing.T) {
	loader := NewPluginLoader(slog.Default())

	// Test with non-existent directory
	plugins, err := loader.DiscoverPlugins([]string{"/nonexistent/dir"})
	if err != nil {
		t.Errorf("Unexpected error when scanning non-existent directory: %v", err)
	}
	if len(plugins) != 0 {
		t.Errorf("Expected 0 plugins from non-existent directory, got %d", len(plugins))
	}

	// Create temporary directory with test files
	tempDir := t.TempDir()
	
	// Create some test files
	testFiles := []string{
		"plugin1.so",
		"plugin2.so",
		"notaplugin.txt",
		"README.md",
	}
	
	for _, file := range testFiles {
		path := filepath.Join(tempDir, file)
		if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", path, err)
		}
	}

	discoveredPlugins, err := loader.DiscoverPlugins([]string{tempDir})
	if err != nil {
		t.Errorf("Unexpected error when scanning directory: %v", err)
	}

	expectedPlugins := 2 // Only .so files should be found
	if len(discoveredPlugins) != expectedPlugins {
		t.Errorf("Expected %d plugins, got %d", expectedPlugins, len(discoveredPlugins))
	}

	// Verify that only .so files were found
	for _, plugin := range discoveredPlugins {
		if filepath.Ext(plugin) != ".so" {
			t.Errorf("Non-.so file found in plugin list: %s", plugin)
		}
	}
}

func TestPluginLoader_GetLoadedPlugins(t *testing.T) {
	loader := NewPluginLoader(slog.Default())

	// Initially should have no plugins
	plugins := loader.GetLoadedPlugins()
	if len(plugins) != 0 {
		t.Errorf("Expected 0 loaded plugins initially, got %d", len(plugins))
	}

	// Test that the returned map is a copy (mutations don't affect internal state)
	plugins["test"] = nil
	pluginsAfter := loader.GetLoadedPlugins()
	if len(pluginsAfter) != 0 {
		t.Error("Mutation of returned map affected internal state")
	}
}

func TestPluginLoader_GetMetadata(t *testing.T) {
	loader := NewPluginLoader(slog.Default())

	// Initially should have no metadata
	metadata := loader.GetMetadata()
	if len(metadata) != 0 {
		t.Errorf("Expected 0 metadata entries initially, got %d", len(metadata))
	}
}
