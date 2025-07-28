package hooks

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"plugin"
	"strings"
	"time"
)

// PluginMetadata contains metadata about a loaded plugin
type PluginMetadata struct {
	Name        string
	Version     string
	Description string
	Path        string
	Loaded      bool
	LoadError   error
}

// PluginLoader handles loading Go plugins (.so files)
type PluginLoader struct {
	plugins map[string]*LoadedPlugin
	logger  *slog.Logger
}

// LoadedPlugin represents a successfully loaded plugin
type LoadedPlugin struct {
	Metadata PluginMetadata
	Plugin   *plugin.Plugin
	Hook     Hook
}

// NewPluginLoader creates a new plugin loader
func NewPluginLoader(logger *slog.Logger) *PluginLoader {
	if logger == nil {
		logger = slog.Default()
	}
	return &PluginLoader{
		plugins: make(map[string]*LoadedPlugin),
		logger:  logger,
	}
}

// LoadPlugin loads a single plugin from the specified path
func (pl *PluginLoader) LoadPlugin(name, path string, config map[string]interface{}) (*LoadedPlugin, error) {
	start := time.Now()
	
	pl.logger.Debug("Starting plugin load",
		"name", name,
		"path", path,
		"config", config)

	// Check if plugin file exists
	statStart := time.Now()
	fileInfo, err := os.Stat(path)
	if os.IsNotExist(err) {
		pl.logger.Error("Plugin file not found",
			"name", name,
			"path", path,
			"stat_duration", time.Since(statStart))
		return nil, fmt.Errorf("plugin file not found: %s", path)
	}
	if err != nil {
		pl.logger.Error("Failed to stat plugin file",
			"name", name,
			"path", path,
			"error", err,
			"stat_duration", time.Since(statStart))
		return nil, fmt.Errorf("failed to stat plugin file %s: %w", path, err)
	}
	
	pl.logger.Debug("Plugin file verified",
		"name", name,
		"path", path,
		"size", fileInfo.Size(),
		"mod_time", fileInfo.ModTime(),
		"stat_duration", time.Since(statStart))

	// Open the plugin
	openStart := time.Now()
	pl.logger.Debug("Opening plugin shared object", "name", name, "path", path)
	
	p, err := plugin.Open(path)
	if err != nil {
		pl.logger.Error("Failed to open plugin shared object",
			"name", name,
			"path", path,
			"error", err,
			"open_duration", time.Since(openStart))
		return nil, fmt.Errorf("failed to open plugin %s: %w", path, err)
	}
	
	pl.logger.Debug("Plugin shared object opened successfully",
		"name", name,
		"path", path,
		"open_duration", time.Since(openStart))

	// Look up the NewHook symbol
	lookupStart := time.Now()
	pl.logger.Debug("Looking up NewHook symbol", "name", name)
	
	newHookSym, err := p.Lookup("NewHook")
	if err != nil {
		pl.logger.Error("Plugin does not export NewHook function",
			"name", name,
			"path", path,
			"error", err,
			"lookup_duration", time.Since(lookupStart))
		return nil, fmt.Errorf("plugin %s does not export NewHook function: %w", path, err)
	}
	
	pl.logger.Debug("NewHook symbol found",
		"name", name,
		"lookup_duration", time.Since(lookupStart))

	// Assert that NewHook is a function that returns a Hook
	newHookFunc, ok := newHookSym.(func() Hook)
	if !ok {
		pl.logger.Error("NewHook function has invalid signature",
			"name", name,
			"path", path,
			"expected", "func() Hook",
			"actual", fmt.Sprintf("%T", newHookSym))
		return nil, fmt.Errorf("plugin %s NewHook function has invalid signature: expected func() Hook", path)
	}
	
	pl.logger.Debug("NewHook function signature validated", "name", name)

	// Create the hook instance
	createStart := time.Now()
	pl.logger.Debug("Creating hook instance", "name", name)
	
	hook := newHookFunc()
	if hook == nil {
		pl.logger.Error("NewHook returned nil",
			"name", name,
			"path", path,
			"create_duration", time.Since(createStart))
		return nil, fmt.Errorf("plugin %s NewHook returned nil", path)
	}

	// Get hook info
	info := hook.Info()
	
	pl.logger.Debug("Hook instance created successfully",
		"name", name,
		"hook_name", info.Name,
		"hook_version", info.Version,
		"hook_description", info.Description,
		"create_duration", time.Since(createStart))

	loadedPlugin := &LoadedPlugin{
		Metadata: PluginMetadata{
			Name:        info.Name,
			Version:     info.Version,
			Description: info.Description,
			Path:        path,
			Loaded:      true,
		},
		Plugin: p,
		Hook:   hook,
	}

	// Store the loaded plugin
	pl.plugins[name] = loadedPlugin

	totalDuration := time.Since(start)
	pl.logger.Info("Plugin loading completed successfully",
		"name", name,
		"hook_name", info.Name,
		"hook_version", info.Version,
		"hook_description", info.Description,
		"path", path,
		"file_size", fileInfo.Size(),
		"total_duration", totalDuration,
		"stat_duration", time.Since(statStart),
		"open_duration", time.Since(openStart),
		"lookup_duration", time.Since(lookupStart),
		"create_duration", time.Since(createStart))

	return loadedPlugin, nil
}

// DiscoverPlugins scans directories for .so files
func (pl *PluginLoader) DiscoverPlugins(dirs []string) ([]string, error) {
	start := time.Now()
	var pluginPaths []string
	var totalFiles, totalDirs int
	
	pl.logger.Debug("Starting plugin discovery",
		"directories", dirs,
		"dir_count", len(dirs))

	for i, dir := range dirs {
		dirStart := time.Now()
		pl.logger.Debug("Scanning directory for plugins",
			"dir", dir,
			"dir_index", i+1,
			"total_dirs", len(dirs))

		// Check if directory exists
		dirInfo, err := os.Stat(dir)
		if os.IsNotExist(err) {
			pl.logger.Debug("Plugin directory does not exist, skipping",
				"dir", dir,
				"dir_check_duration", time.Since(dirStart))
			continue
		}
		if err != nil {
			pl.logger.Warn("Error checking plugin directory",
				"dir", dir,
				"error", err,
				"dir_check_duration", time.Since(dirStart))
			continue
		}
		
		if !dirInfo.IsDir() {
			pl.logger.Warn("Plugin path is not a directory, skipping",
				"path", dir,
				"dir_check_duration", time.Since(dirStart))
			continue
		}
		
		pl.logger.Debug("Plugin directory found",
			"dir", dir,
			"mod_time", dirInfo.ModTime(),
			"dir_check_duration", time.Since(dirStart))

		// Walk through the directory
		var dirFiles, dirPlugins int
		walkStart := time.Now()
		
		err = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				pl.logger.Warn("Error walking plugin directory entry",
					"dir", dir,
					"path", path,
					"error", err)
				return nil // Continue walking
			}

			if d.IsDir() {
				totalDirs++
				pl.logger.Debug("Scanning subdirectory",
					"dir", dir,
					"subdir", path)
				return nil
			}
			
			totalFiles++
			dirFiles++

			// Only consider .so files
			if strings.HasSuffix(path, ".so") {
				fileInfo, statErr := d.Info()
				if statErr != nil {
					pl.logger.Warn("Failed to get file info for plugin",
						"path", path,
						"error", statErr)
				}
				
				pluginPaths = append(pluginPaths, path)
				dirPlugins++
				
				logFields := []interface{}{
					"path", path,
					"dir", dir,
					"filename", d.Name(),
				}
				if fileInfo != nil {
					logFields = append(logFields,
						"size", fileInfo.Size(),
						"mod_time", fileInfo.ModTime())
				}
				
				pl.logger.Debug("Found plugin file", logFields...)
			} else {
				pl.logger.Debug("Skipping non-plugin file",
					"path", path,
					"dir", dir,
					"filename", d.Name())
			}

			return nil
		})

		walkDuration := time.Since(walkStart)
		dirDuration := time.Since(dirStart)

		if err != nil {
			pl.logger.Error("Error scanning plugin directory",
				"dir", dir,
				"error", err,
				"walk_duration", walkDuration,
				"dir_duration", dirDuration)
			return nil, fmt.Errorf("failed to scan plugin directory %s: %w", dir, err)
		}
		
		pl.logger.Debug("Directory scan completed",
			"dir", dir,
			"plugins_found", dirPlugins,
			"total_files", dirFiles,
			"walk_duration", walkDuration,
			"dir_duration", dirDuration)
	}

	totalDuration := time.Since(start)
	pl.logger.Info("Plugin discovery completed",
		"found_plugins", len(pluginPaths),
		"scanned_directories", len(dirs),
		"total_files", totalFiles,
		"total_dirs", totalDirs,
		"duration", totalDuration,
		"plugin_paths", pluginPaths)
		
	return pluginPaths, nil
}

// GetLoadedPlugins returns a map of all loaded plugins
func (pl *PluginLoader) GetLoadedPlugins() map[string]*LoadedPlugin {
	result := make(map[string]*LoadedPlugin)
	for k, v := range pl.plugins {
		result[k] = v
	}
	return result
}

// GetPlugin returns a specific loaded plugin by name
func (pl *PluginLoader) GetPlugin(name string) (*LoadedPlugin, bool) {
	plugin, exists := pl.plugins[name]
	return plugin, exists
}

// GetMetadata returns metadata for all plugins (loaded and failed)
func (pl *PluginLoader) GetMetadata() []PluginMetadata {
	var metadata []PluginMetadata
	for _, p := range pl.plugins {
		metadata = append(metadata, p.Metadata)
	}
	return metadata
}

// LoadError represents a plugin that failed to load
type LoadError struct {
	Name string
	Path string
	Err  error
}

// LoadPluginsFromConfig loads plugins based on configuration
func (pl *PluginLoader) LoadPluginsFromConfig(pluginDirs []string, plugins map[string]interface{}) ([]Hook, []LoadError) {
	var hooks []Hook
	var loadErrors []LoadError

	// Discover plugins from directories
	if len(pluginDirs) > 0 {
		discoveredPaths, err := pl.DiscoverPlugins(pluginDirs)
		if err != nil {
			pl.logger.Error("Failed to discover plugins", "error", err)
		} else {
			// Load discovered plugins
			for _, path := range discoveredPaths {
				// Generate name from filename
				name := strings.TrimSuffix(filepath.Base(path), ".so")
				
				loadedPlugin, err := pl.LoadPlugin(name, path, nil)
				if err != nil {
					pl.logger.Error("Failed to load discovered plugin", "name", name, "path", path, "error", err)
					loadErrors = append(loadErrors, LoadError{Name: name, Path: path, Err: err})
					continue
				}
				
				hooks = append(hooks, loadedPlugin.Hook)
			}
		}
	}

	// Load explicitly configured plugins
	for name, configData := range plugins {
		pluginConfig, ok := configData.(map[string]interface{})
		if !ok {
			err := fmt.Errorf("invalid plugin configuration for %s", name)
			pl.logger.Error("Invalid plugin configuration", "name", name, "error", err)
			loadErrors = append(loadErrors, LoadError{Name: name, Err: err})
			continue
		}

		// Check if disabled
		if disabled, ok := pluginConfig["disabled"].(bool); ok && disabled {
			pl.logger.Debug("Plugin disabled, skipping", "name", name)
			continue
		}

		// Get plugin path
		path, ok := pluginConfig["path"].(string)
		if !ok {
			err := fmt.Errorf("plugin %s missing path configuration", name)
			pl.logger.Error("Plugin missing path", "name", name, "error", err)
			loadErrors = append(loadErrors, LoadError{Name: name, Err: err})
			continue
		}

		// Get plugin-specific config
		config, _ := pluginConfig["config"].(map[string]interface{})

		loadedPlugin, err := pl.LoadPlugin(name, path, config)
		if err != nil {
			pl.logger.Error("Failed to load configured plugin", "name", name, "path", path, "error", err)
			loadErrors = append(loadErrors, LoadError{Name: name, Path: path, Err: err})
			continue
		}

		hooks = append(hooks, loadedPlugin.Hook)
	}

	pl.logger.Info("Plugin loading completed",
		"loaded", len(hooks),
		"errors", len(loadErrors))

	return hooks, loadErrors
}
