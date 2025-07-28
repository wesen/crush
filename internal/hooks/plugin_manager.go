package hooks

import (
	"fmt"
	"log/slog"
	"time"
)

// PluginManager manages the lifecycle of hook plugins
type PluginManager struct {
	loader          *PluginLoader
	transformMgr    *TransformManager
	eventMgr        *Manager
	serviceRegistry ServiceRegistry
	logger          *slog.Logger
	
	// Track loaded plugins and their hooks
	loadedHooks   []Hook
	loadErrors    []LoadError
}

// NewPluginManager creates a new plugin manager
func NewPluginManager(transformMgr *TransformManager, eventMgr *Manager, serviceRegistry ServiceRegistry, logger *slog.Logger) *PluginManager {
	if logger == nil {
		logger = slog.Default()
	}
	
	return &PluginManager{
		loader:          NewPluginLoader(logger),
		transformMgr:    transformMgr,
		eventMgr:        eventMgr,
		serviceRegistry: serviceRegistry,
		logger:          logger,
		loadedHooks:     make([]Hook, 0),
		loadErrors:      make([]LoadError, 0),
	}
}

// LoadPluginsFromConfig loads and registers plugins from configuration
func (pm *PluginManager) LoadPluginsFromConfig(pluginDirs []string, pluginConfigs map[string]interface{}) error {
	start := time.Now()
	
	pm.logger.Info("Starting plugin loading",
		"plugin_dirs", len(pluginDirs),
		"plugin_dirs_list", pluginDirs,
		"configured_plugins", len(pluginConfigs))

	// Log detailed plugin directory information
	for i, dir := range pluginDirs {
		pm.logger.Debug("Plugin directory to scan",
			"index", i,
			"directory", dir)
	}
	
	// Log configured plugins
	for name, config := range pluginConfigs {
		pm.logger.Debug("Configured plugin",
			"name", name,
			"config", config)
	}

	// Load plugins using the loader
	loadStart := time.Now()
	hooks, loadErrors := pm.loader.LoadPluginsFromConfig(pluginDirs, pluginConfigs)
	loadDuration := time.Since(loadStart)
	
	pm.logger.Debug("Plugin loading phase completed",
		"duration", loadDuration,
		"discovered_hooks", len(hooks),
		"load_errors", len(loadErrors))
	
	// Store load errors for reporting
	pm.loadErrors = append(pm.loadErrors, loadErrors...)
	
	// Register successfully loaded hooks
	registerStart := time.Now()
	successfulRegistrations := 0
	for _, hook := range hooks {
		hookStart := time.Now()
		if err := pm.RegisterHook(hook); err != nil {
			pm.logger.Error("Failed to register loaded plugin hook",
				"hook", hook.Info().Name,
				"error", err,
				"registration_duration", time.Since(hookStart))
			pm.loadErrors = append(pm.loadErrors, LoadError{
				Name: hook.Info().Name,
				Err:  err,
			})
			continue
		}
		pm.loadedHooks = append(pm.loadedHooks, hook)
		successfulRegistrations++
		
		pm.logger.Debug("Hook registered successfully",
			"hook", hook.Info().Name,
			"registration_duration", time.Since(hookStart))
	}
	registerDuration := time.Since(registerStart)

	totalDuration := time.Since(start)
	pm.logger.Info("Plugin loading completed",
		"total_duration", totalDuration,
		"load_duration", loadDuration,
		"register_duration", registerDuration,
		"loaded_hooks", len(pm.loadedHooks),
		"successful_registrations", successfulRegistrations,
		"load_errors", len(pm.loadErrors))

	// Log detailed error information
	for _, loadErr := range pm.loadErrors {
		pm.logger.Error("Plugin load error details",
			"name", loadErr.Name,
			"path", loadErr.Path,
			"error", loadErr.Err)
	}

	return nil
}

// RegisterHook registers a hook with appropriate managers based on its interfaces
func (pm *PluginManager) RegisterHook(hook Hook) error {
	start := time.Now()
	info := hook.Info()
	
	pm.logger.Debug("Starting hook registration",
		"name", info.Name,
		"version", info.Version,
		"description", info.Description)

	// Analyze hook interfaces
	var implementedInterfaces []string
	var isTransformHook, isEventHook bool
	
	// Check all possible hook interfaces
	if _, ok := hook.(HookInitializer); ok {
		implementedInterfaces = append(implementedInterfaces, "HookInitializer")
	}
	if _, ok := hook.(TransformToolHook); ok {
		implementedInterfaces = append(implementedInterfaces, "TransformToolHook")
		isTransformHook = true
	}
	if _, ok := hook.(TransformSessionHook); ok {
		implementedInterfaces = append(implementedInterfaces, "TransformSessionHook")
		isTransformHook = true
	}
	if _, ok := hook.(BeforeLLMCaller); ok {
		implementedInterfaces = append(implementedInterfaces, "BeforeLLMCaller")
		isEventHook = true
	}
	if _, ok := hook.(AfterLLMInferencer); ok {
		implementedInterfaces = append(implementedInterfaces, "AfterLLMInferencer")
		isEventHook = true
	}
	if _, ok := hook.(BeforeToolCaller); ok {
		implementedInterfaces = append(implementedInterfaces, "BeforeToolCaller")
		isEventHook = true
	}
	if _, ok := hook.(AfterToolResulter); ok {
		implementedInterfaces = append(implementedInterfaces, "AfterToolResulter")
		isEventHook = true
	}
	
	pm.logger.Debug("Hook interface analysis",
		"name", info.Name,
		"implemented_interfaces", implementedInterfaces,
		"is_transform_hook", isTransformHook,
		"is_event_hook", isEventHook)

	// Initialize hook if it supports initialization
	if initializer, ok := hook.(HookInitializer); ok {
		initStart := time.Now()
		pm.logger.Debug("Initializing hook", "name", info.Name)
		
		if err := initializer.Initialize(nil, pm.serviceRegistry); err != nil {
			pm.logger.Error("Hook initialization failed",
				"name", info.Name,
				"error", err,
				"init_duration", time.Since(initStart))
			return fmt.Errorf("failed to initialize hook %s: %w", info.Name, err)
		}
		
		pm.logger.Debug("Hook initialized successfully",
			"name", info.Name,
			"init_duration", time.Since(initStart))
	}

	// Register with transform manager for transform hooks
	if isTransformHook {
		transformStart := time.Now()
		pm.logger.Debug("Registering hook with transform manager", "name", info.Name)
		
		if err := pm.transformMgr.Add(hook); err != nil {
			pm.logger.Error("Failed to register hook with transform manager",
				"name", info.Name,
				"error", err,
				"transform_registration_duration", time.Since(transformStart))
			return fmt.Errorf("failed to add transform hook %s: %w", info.Name, err)
		}
		
		pm.logger.Debug("Hook registered with transform manager successfully",
			"name", info.Name,
			"transform_registration_duration", time.Since(transformStart))
	}

	// Register with event manager for event hooks
	if isEventHook {
		eventStart := time.Now()
		pm.logger.Debug("Registering hook with event manager", "name", info.Name)
		
		pm.eventMgr.Add(hook)
		
		pm.logger.Debug("Hook registered with event manager successfully",
			"name", info.Name,
			"event_registration_duration", time.Since(eventStart))
	}

	// Warning for hooks that don't implement known interfaces
	if !isTransformHook && !isEventHook {
		pm.logger.Warn("Hook does not implement any known hook interfaces",
			"name", info.Name,
			"description", info.Description,
			"implemented_interfaces", implementedInterfaces)
	}

	totalDuration := time.Since(start)
	pm.logger.Info("Hook registration completed successfully",
		"name", info.Name,
		"version", info.Version,
		"description", info.Description,
		"transform_hook", isTransformHook,
		"event_hook", isEventHook,
		"implemented_interfaces", implementedInterfaces,
		"registration_duration", totalDuration)

	return nil
}

// GetLoadedHooks returns all successfully loaded hooks
func (pm *PluginManager) GetLoadedHooks() []Hook {
	result := make([]Hook, len(pm.loadedHooks))
	copy(result, pm.loadedHooks)
	return result
}

// GetLoadErrors returns all plugin loading errors
func (pm *PluginManager) GetLoadErrors() []LoadError {
	result := make([]LoadError, len(pm.loadErrors))
	copy(result, pm.loadErrors)
	return result
}

// GetPluginMetadata returns metadata for all plugins (loaded and failed)
func (pm *PluginManager) GetPluginMetadata() []PluginMetadata {
	return pm.loader.GetMetadata()
}

// GetPluginByName returns a specific loaded plugin by name
func (pm *PluginManager) GetPluginByName(name string) (*LoadedPlugin, bool) {
	return pm.loader.GetPlugin(name)
}

// GetStatistics returns loading statistics
func (pm *PluginManager) GetStatistics() map[string]interface{} {
	loadedPlugins := pm.loader.GetLoadedPlugins()
	
	stats := map[string]interface{}{
		"total_loaded":   len(pm.loadedHooks),
		"total_errors":   len(pm.loadErrors),
		"loaded_plugins": make(map[string]interface{}),
	}

	for name, plugin := range loadedPlugins {
		info := plugin.Hook.Info()
		stats["loaded_plugins"].(map[string]interface{})[name] = map[string]interface{}{
			"name":        info.Name,
			"version":     info.Version,
			"description": info.Description,
			"path":        plugin.Metadata.Path,
		}
	}

	return stats
}

// LoadSinglePlugin loads a single plugin from a path (useful for testing)
func (pm *PluginManager) LoadSinglePlugin(name, path string, config map[string]interface{}) error {
	pm.logger.Info("Loading single plugin", "name", name, "path", path)

	loadedPlugin, err := pm.loader.LoadPlugin(name, path, config)
	if err != nil {
		pm.logger.Error("Failed to load single plugin", "name", name, "path", path, "error", err)
		pm.loadErrors = append(pm.loadErrors, LoadError{Name: name, Path: path, Err: err})
		return err
	}

	// Register the hook
	if err := pm.RegisterHook(loadedPlugin.Hook); err != nil {
		pm.logger.Error("Failed to register single plugin hook", "name", name, "error", err)
		pm.loadErrors = append(pm.loadErrors, LoadError{Name: name, Path: path, Err: err})
		return err
	}

	pm.loadedHooks = append(pm.loadedHooks, loadedPlugin.Hook)
	pm.logger.Info("Successfully loaded and registered single plugin", "name", name, "path", path)

	return nil
}
