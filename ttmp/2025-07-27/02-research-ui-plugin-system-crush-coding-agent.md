# UI Plugin System Research for Crush Coding Agent

## Executive Summary

This document contains comprehensive research for implementing a UI plugin system in the Crush coding agent that allows loadable Go plugins to add custom conversation widgets, menu entries, and sidebar visualizations. The system will enable experimentation with different modalities while maintaining the core TUI architecture.

## Table of Contents

1. [Current UI Architecture Analysis](#current-ui-architecture-analysis)
2. [Plugin Integration Points](#plugin-integration-points)
3. [UI Plugin Interface Design](#ui-plugin-interface-design)
4. [Widget Registration System](#widget-registration-system)
5. [Runtime Behavior Design](#runtime-behavior-design)
6. [Menu and Navigation Extensions](#menu-and-navigation-extensions)
7. [Sidebar Plugin Architecture](#sidebar-plugin-architecture)
8. [Security and Isolation](#security-and-isolation)
9. [Implementation Roadmap](#implementation-roadmap)

## Current UI Architecture Analysis

### Core TUI Architecture

The Crush TUI is built on the Bubble Tea framework with a hierarchical component structure:

#### 1. Application Model (`internal/tui/tui.go`)
- **Main Controller**: `appModel` manages pages, dialogs, status bar, and global state
- **Event Routing**: Routes messages between components and handles global keybindings
- **Dialog System**: Manages modal overlays with stack-based navigation
- **Window Management**: Handles resize events and coordinate layout updates

#### 2. Page System (`internal/tui/page/chat/chat.go`)
- **Chat Page**: Primary interface with panel-based layout (chat, editor, sidebar, splash)
- **Layout Modes**: Supports compact and full modes with responsive breakpoints
- **Focus Management**: Tab-based navigation between panels with proper focus handling
- **Session Management**: Handles session switching and state persistence

#### 3. Component Architecture
```
internal/tui/components/
├── chat/                    # Chat-related components
│   ├── chat.go             # Message list with virtualization
│   ├── editor/             # Message input component  
│   ├── header/             # Chat header for compact mode
│   ├── sidebar/            # Session info and file tracking
│   ├── messages/           # Individual message rendering
│   └── splash/             # Initial setup screens
├── dialogs/                # Modal dialog system
│   ├── dialogs.go          # Dialog manager with stack
│   ├── commands/           # Command palette dialog
│   ├── sessions/           # Session selection dialog
│   ├── models/             # Model selection dialog
│   └── filepicker/         # File picker dialog
├── core/                   # Reusable UI primitives
│   ├── layout/             # Layout interfaces
│   ├── list/               # Virtualized list component
│   └── status/             # Status bar component
└── completions/            # Auto-completion system
```

#### 4. Key Design Patterns
- **Bubble Tea Models**: Each component implements `Init()`, `Update()`, `View()`
- **Interface Composition**: Components implement `layout.Sizeable`, `layout.Focusable`, etc.
- **Message Passing**: Components communicate via typed messages (e.g., `SessionSelectedMsg`)
- **Event Subscription**: Real-time updates via pub/sub pattern with app services
- **Layer Composition**: UI renders as composable lipgloss layers

### Message and Conversation System

#### Message Components (`internal/tui/components/chat/messages/`)
- **Message Rendering**: `messages.go` - Handles user/assistant message display
- **Tool Visualization**: `tool.go` - Specialized rendering for tool calls and results
- **Renderer System**: `renderer.go` - Pluggable renderers for different tool types
- **Real-time Updates**: Messages update incrementally during streaming

#### Chat List (`internal/tui/components/chat/chat.go`)
- **Virtualization**: Efficient rendering of large conversation histories
- **Dynamic Updates**: Real-time message updates during LLM generation
- **Tool Call Tracking**: Manages tool call lifecycle and nested executions
- **Session Switching**: Loads and displays messages for different sessions

### Sidebar and Menu Architecture

#### Sidebar System (`internal/tui/components/chat/sidebar/sidebar.go`)
- **Multi-Section Layout**: Modified Files, LSPs, MCPs with dynamic sizing
- **Responsive Design**: Compact/full modes with horizontal/vertical layouts
- **Real-time Status**: File changes, LSP diagnostics, service health
- **Session Context**: Shows session-specific file modifications and statistics

#### Dialog and Menu System (`internal/tui/components/dialogs/`)
- **Dialog Manager**: Stack-based modal system with proper focus management
- **Command Palette**: Fuzzy-searchable command interface
- **Session Management**: Visual session selection with metadata
- **Model Selection**: Provider and model configuration interface

## Plugin Integration Points

Based on the architecture analysis, we identified three primary integration points for UI plugins:

### 1. Conversation Widget Plugins
**Integration Point**: Message rendering system in `internal/tui/components/chat/messages/`
- **Before**: `messages.NewMessageCmp()` and `messages.NewToolCallCmp()`
- **Custom Renderers**: Replace or extend the existing renderer system
- **Real-time Updates**: Integrate with streaming message updates

### 2. Menu Entry Plugins  
**Integration Point**: Command system and dialog management
- **Commands Dialog**: Extend `internal/tui/components/dialogs/commands/`
- **Global Keybindings**: Add entries to main keymap in `internal/tui/tui.go`
- **Custom Dialogs**: Register new dialog types in dialog system

### 3. Sidebar Visualization Plugins
**Integration Point**: Sidebar sections in `internal/tui/components/chat/sidebar/`
- **Custom Sections**: Add new sections alongside Files, LSPs, MCPs
- **Dynamic Content**: Integrate with real-time data sources
- **Layout Management**: Participate in responsive layout system

## UI Plugin Interface Design

### Core Plugin Interfaces

```go
package uiplugins

import (
    "context"
    tea "github.com/charmbracelet/bubbletea/v2"
    "github.com/charmbracelet/crush/internal/session"
    "github.com/charmbracelet/crush/internal/message"
    "github.com/charmbracelet/crush/internal/tui/util"
    "github.com/charmbracelet/crush/internal/tui/components/core/layout"
)

// PluginContext provides context for UI plugin execution
type PluginContext struct {
    SessionID   string
    Session     session.Session
    Width       int
    Height      int
    CompactMode bool
    Theme       ThemeInfo
    Config      map[string]interface{}
}

// ThemeInfo provides theme information to plugins
type ThemeInfo struct {
    Primary    string
    Secondary  string
    Success    string
    Warning    string
    Error      string
    Muted      string
}

// ConversationWidget defines the interface for custom message widgets
type ConversationWidget interface {
    // Info returns metadata about the widget
    Info() WidgetInfo
    
    // CanRender determines if this widget can render the given message
    CanRender(msg message.Message) bool
    
    // Render creates a widget component for the message
    Render(ctx PluginContext, msg message.Message) (util.Model, error)
    
    // Priority returns rendering priority (higher = preferred)
    Priority() int
}

// SidebarPlugin defines the interface for sidebar visualizations
type SidebarPlugin interface {
    // Info returns metadata about the plugin
    Info() PluginInfo
    
    // CreateSection creates a sidebar section component
    CreateSection(ctx PluginContext) (SidebarSection, error)
    
    // SupportsCompactMode indicates if plugin works in compact mode
    SupportsCompactMode() bool
}

// MenuPlugin defines the interface for custom menu entries
type MenuPlugin interface {
    // Info returns metadata about the plugin
    Info() PluginInfo
    
    // Commands returns menu commands provided by this plugin
    Commands(ctx PluginContext) []Command
    
    // CreateDialog creates a dialog for a command (optional)
    CreateDialog(ctx PluginContext, commandID string) (util.Model, error)
    
    // ExecuteCommand handles direct command execution
    ExecuteCommand(ctx PluginContext, commandID string, args map[string]string) tea.Cmd
}

// Combined plugin interface for multi-purpose plugins
type UIPlugin interface {
    // Core plugin interface
    Info() PluginInfo
    Initialize(config map[string]interface{}) error
    Shutdown() error
    
    // Optional capability interfaces
    ConversationWidget // embedded if plugin provides conversation widgets
    SidebarPlugin     // embedded if plugin provides sidebar sections
    MenuPlugin        // embedded if plugin provides menu commands
}

// PluginInfo provides metadata about a UI plugin
type PluginInfo struct {
    ID          string
    Name        string
    Version     string
    Description string
    Author      string
    Capabilities []PluginCapability
}

type PluginCapability string

const (
    CapabilityConversationWidget PluginCapability = "conversation_widget"
    CapabilitySidebarSection     PluginCapability = "sidebar_section"  
    CapabilityMenuCommands       PluginCapability = "menu_commands"
)

// Widget and component interfaces
type WidgetInfo struct {
    Name        string
    Description string
    MessageTypes []message.MessageRole // Which message types this widget handles
    ToolNames   []string             // Which tool names this widget handles (for tool calls)
}

type SidebarSection interface {
    util.Model
    layout.Sizeable
    
    // Title returns the section title
    Title() string
    
    // Content returns the current section content
    Content() []SidebarItem
    
    // SetSession updates the section for a new session
    SetSession(session session.Session) tea.Cmd
    
    // Priority determines section ordering
    Priority() int
}

type SidebarItem struct {
    Title       string
    Description string
    Status      string
    IconColor   string
    Extra       string
}

type Command struct {
    ID          string
    Name        string
    Description string
    Category    string
    KeyBinding  string // Optional keyboard shortcut
    Args        []CommandArg
}

type CommandArg struct {
    Name        string
    Description string
    Type        ArgType
    Required    bool
    Default     string
}

type ArgType string

const (
    ArgTypeString   ArgType = "string"
    ArgTypeInteger  ArgType = "integer"
    ArgTypeBoolean  ArgType = "boolean"
    ArgTypeFile     ArgType = "file"
)
```

### Plugin Registration and Discovery

```go
// PluginRegistry manages UI plugin lifecycle
type PluginRegistry interface {
    // RegisterPlugin adds a plugin to the registry
    RegisterPlugin(plugin UIPlugin) error
    
    // UnregisterPlugin removes a plugin from the registry
    UnregisterPlugin(pluginID string) error
    
    // GetConversationWidgets returns all conversation widget plugins
    GetConversationWidgets() []ConversationWidget
    
    // GetSidebarPlugins returns all sidebar plugins
    GetSidebarPlugins() []SidebarPlugin
    
    // GetMenuPlugins returns all menu plugins
    GetMenuPlugins() []MenuPlugin
    
    // LoadFromDirectory loads plugins from a directory
    LoadFromDirectory(dir string) error
}

// PluginManager handles plugin discovery and integration
type PluginManager struct {
    registry    PluginRegistry
    pluginDirs  []string
    config      map[string]map[string]interface{}
    
    // Integration points
    widgetRegistry   *ConversationWidgetRegistry
    sidebarManager   *SidebarManager
    commandManager   *CommandManager
}

func NewPluginManager(config UIPluginConfig) *PluginManager {
    return &PluginManager{
        registry:         NewPluginRegistry(),
        pluginDirs:       config.PluginDirs,
        config:          config.Plugins,
        widgetRegistry:  NewConversationWidgetRegistry(),
        sidebarManager:  NewSidebarManager(),
        commandManager:  NewCommandManager(),
    }
}

func (pm *PluginManager) LoadAllPlugins() error {
    for _, dir := range pm.pluginDirs {
        if err := pm.registry.LoadFromDirectory(dir); err != nil {
            // Log error but continue loading from other directories
            continue
        }
    }
    
    // Register plugins with their respective managers
    for _, plugin := range pm.registry.GetConversationWidgets() {
        pm.widgetRegistry.Register(plugin)
    }
    
    for _, plugin := range pm.registry.GetSidebarPlugins() {
        pm.sidebarManager.Register(plugin)
    }
    
    for _, plugin := range pm.registry.GetMenuPlugins() {
        pm.commandManager.Register(plugin)
    }
    
    return nil
}
```

## Widget Registration System

### Conversation Widget Integration

```go
// ConversationWidgetRegistry manages message rendering plugins
type ConversationWidgetRegistry struct {
    widgets    []ConversationWidget
    renderers  map[string]ConversationWidget // Tool name -> widget
    fallback   ConversationWidget           // Default widget
}

func (cwr *ConversationWidgetRegistry) Register(widget ConversationWidget) {
    cwr.widgets = append(cwr.widgets, widget)
    
    // Sort by priority (higher priority first)
    sort.Slice(cwr.widgets, func(i, j int) bool {
        return cwr.widgets[i].Priority() > cwr.widgets[j].Priority()
    })
    
    // Index by tool names for fast lookup
    info := widget.Info()
    for _, toolName := range info.ToolNames {
        cwr.renderers[toolName] = widget
    }
}

func (cwr *ConversationWidgetRegistry) RenderMessage(ctx PluginContext, msg message.Message) util.Model {
    // For tool calls, check specific tool renderers first
    if len(msg.ToolCalls()) > 0 {
        for _, tc := range msg.ToolCalls() {
            if widget, ok := cwr.renderers[tc.Name]; ok {
                if widget.CanRender(msg) {
                    if component, err := widget.Render(ctx, msg); err == nil {
                        return component
                    }
                }
            }
        }
    }
    
    // Try all registered widgets in priority order
    for _, widget := range cwr.widgets {
        if widget.CanRender(msg) {
            if component, err := widget.Render(ctx, msg); err == nil {
                return component
            }
        }
    }
    
    // Fall back to default renderer
    component, _ := cwr.fallback.Render(ctx, msg)
    return component
}

// Integration with existing message system
// In internal/tui/components/chat/messages/messages.go:

func NewMessageCmp(msg message.Message, options ...MessageOption) MessageCmp {
    // Check if plugin registry has a custom widget for this message
    if pluginManager := GetGlobalPluginManager(); pluginManager != nil {
        ctx := PluginContext{
            SessionID: extractSessionID(msg),
            Width:     defaultWidth,
            Height:    defaultHeight,
            Theme:     getCurrentTheme(),
        }
        
        if customWidget := pluginManager.widgetRegistry.RenderMessage(ctx, msg); customWidget != nil {
            return &pluginMessageWrapper{
                widget: customWidget,
                msg:    msg,
            }
        }
    }
    
    // Fall back to default message component
    return &messageCmp{
        message: msg,
        // ... existing initialization
    }
}
```

### Command System Integration

```go
// CommandManager handles menu command plugins
type CommandManager struct {
    commands map[string]Command
    plugins  map[string]MenuPlugin
    keyMap   map[string]string // Key binding -> command ID
}

func (cm *CommandManager) Register(plugin MenuPlugin) {
    ctx := PluginContext{} // Base context for command discovery
    commands := plugin.Commands(ctx)
    
    for _, cmd := range commands {
        cm.commands[cmd.ID] = cmd
        cm.plugins[cmd.ID] = plugin
        
        if cmd.KeyBinding != "" {
            cm.keyMap[cmd.KeyBinding] = cmd.ID
        }
    }
}

func (cm *CommandManager) GetAllCommands(ctx PluginContext) []Command {
    var allCommands []Command
    
    // Get commands from all plugins with current context
    for _, plugin := range cm.plugins {
        commands := plugin.Commands(ctx)
        allCommands = append(allCommands, commands...)
    }
    
    return allCommands
}

func (cm *CommandManager) ExecuteCommand(ctx PluginContext, commandID string, args map[string]string) tea.Cmd {
    if plugin, ok := cm.plugins[commandID]; ok {
        return plugin.ExecuteCommand(ctx, commandID, args)
    }
    return nil
}

// Integration with commands dialog
// In internal/tui/components/dialogs/commands/commands.go:

func (m *commandsDialogCmp) loadCommands() {
    // Get built-in commands
    builtinCommands := getBuiltinCommands()
    
    // Get plugin commands
    if pluginManager := GetGlobalPluginManager(); pluginManager != nil {
        ctx := PluginContext{
            SessionID:   m.sessionID,
            CompactMode: m.compactMode,
            Theme:       getCurrentTheme(),
        }
        pluginCommands := pluginManager.commandManager.GetAllCommands(ctx)
        allCommands = append(builtinCommands, pluginCommands...)
    }
    
    // Sort commands by category and name
    sort.Slice(allCommands, func(i, j int) bool {
        if allCommands[i].Category != allCommands[j].Category {
            return allCommands[i].Category < allCommands[j].Category
        }
        return allCommands[i].Name < allCommands[j].Name
    })
    
    m.commands = allCommands
}
```

## Runtime Behavior Design

### Plugin Lifecycle Management

```go
// PluginLifecycleManager handles plugin initialization and shutdown
type PluginLifecycleManager struct {
    plugins     map[string]UIPlugin
    initialized map[string]bool
    failed      map[string]error
}

func (plm *PluginLifecycleManager) InitializePlugin(plugin UIPlugin, config map[string]interface{}) error {
    pluginID := plugin.Info().ID
    
    // Initialize plugin
    if err := plugin.Initialize(config); err != nil {
        plm.failed[pluginID] = err
        return fmt.Errorf("failed to initialize plugin %s: %w", pluginID, err)
    }
    
    plm.plugins[pluginID] = plugin
    plm.initialized[pluginID] = true
    
    // Register with appropriate managers based on capabilities
    info := plugin.Info()
    for _, capability := range info.Capabilities {
        switch capability {
        case CapabilityConversationWidget:
            if widget, ok := plugin.(ConversationWidget); ok {
                GetGlobalPluginManager().widgetRegistry.Register(widget)
            }
        case CapabilitySidebarSection:
            if sidebar, ok := plugin.(SidebarPlugin); ok {
                GetGlobalPluginManager().sidebarManager.Register(sidebar)
            }
        case CapabilityMenuCommands:
            if menu, ok := plugin.(MenuPlugin); ok {
                GetGlobalPluginManager().commandManager.Register(menu)
            }
        }
    }
    
    return nil
}

func (plm *PluginLifecycleManager) ShutdownAll() {
    for pluginID, plugin := range plm.plugins {
        if err := plugin.Shutdown(); err != nil {
            // Log error but continue shutdown
            log.Printf("Error shutting down plugin %s: %v", pluginID, err)
        }
    }
}
```

### Dynamic Plugin Loading

```go
// PluginLoader handles dynamic loading of Go plugins
type PluginLoader struct {
    loadedPlugins map[string]*plugin.Plugin
}

func (pl *PluginLoader) LoadPlugin(pluginPath string) (UIPlugin, error) {
    // Load the plugin
    p, err := plugin.Open(pluginPath)
    if err != nil {
        return nil, fmt.Errorf("failed to load plugin %s: %w", pluginPath, err)
    }
    
    // Look up the NewUIPlugin symbol
    symbol, err := p.Lookup("NewUIPlugin")
    if err != nil {
        return nil, fmt.Errorf("plugin %s does not export NewUIPlugin: %w", pluginPath, err)
    }
    
    // Assert correct function signature
    newPluginFunc, ok := symbol.(func() (UIPlugin, error))
    if !ok {
        return nil, fmt.Errorf("plugin %s NewUIPlugin has incorrect signature", pluginPath)
    }
    
    // Create plugin instance
    plugin, err := newPluginFunc()
    if err != nil {
        return nil, fmt.Errorf("failed to create plugin from %s: %w", pluginPath, err)
    }
    
    pl.loadedPlugins[pluginPath] = p
    return plugin, nil
}

func (pl *PluginLoader) DiscoverPlugins(dir string) ([]UIPlugin, error) {
    var plugins []UIPlugin
    
    files, err := filepath.Glob(filepath.Join(dir, "*.so"))
    if err != nil {
        return nil, err
    }
    
    for _, file := range files {
        if plugin, err := pl.LoadPlugin(file); err == nil {
            plugins = append(plugins, plugin)
        } else {
            // Log error but continue loading other plugins
            log.Printf("Failed to load plugin %s: %v", file, err)
        }
    }
    
    return plugins, nil
}
```

### Hot Reloading Support

```go
// PluginWatcher enables hot reloading during development
type PluginWatcher struct {
    watcher    *fsnotify.Watcher
    manager    *PluginManager
    pluginDirs []string
}

func (pw *PluginWatcher) WatchForChanges() error {
    for _, dir := range pw.pluginDirs {
        if err := pw.watcher.Add(dir); err != nil {
            return err
        }
    }
    
    go func() {
        for {
            select {
            case event, ok := <-pw.watcher.Events:
                if !ok {
                    return
                }
                
                if event.Op&fsnotify.Write == fsnotify.Write {
                    if strings.HasSuffix(event.Name, ".so") {
                        pw.reloadPlugin(event.Name)
                    }
                }
                
            case err, ok := <-pw.watcher.Errors:
                if !ok {
                    return
                }
                log.Printf("Plugin watcher error: %v", err)
            }
        }
    }()
    
    return nil
}

func (pw *PluginWatcher) reloadPlugin(pluginPath string) {
    // Extract plugin ID from path
    pluginID := extractPluginID(pluginPath)
    
    // Unregister existing plugin
    pw.manager.UnregisterPlugin(pluginID)
    
    // Wait a moment for file operations to complete
    time.Sleep(100 * time.Millisecond)
    
    // Reload the plugin
    if plugin, err := pw.manager.LoadPlugin(pluginPath); err == nil {
        log.Printf("Reloaded plugin: %s", pluginID)
    } else {
        log.Printf("Failed to reload plugin %s: %v", pluginID, err)
    }
}
```

## Menu and Navigation Extensions

### Global Keymap Integration

```go
// In internal/tui/tui.go - extend handleKeyPressMsg:

func (a *appModel) handleKeyPressMsg(msg tea.KeyPressMsg) tea.Cmd {
    // Check plugin commands first
    if a.pluginManager != nil {
        ctx := PluginContext{
            SessionID:   a.selectedSessionID,
            CompactMode: a.isCompactMode(),
            Theme:       getCurrentTheme(),
        }
        
        if cmd := a.pluginManager.commandManager.HandleKeyPress(ctx, msg); cmd != nil {
            return cmd
        }
    }
    
    // Existing key handling logic...
    switch {
    case key.Matches(msg, a.keyMap.Commands):
        // Open command palette with plugin commands included
        return a.openCommandPalette()
    // ... rest of existing cases
    }
}

func (a *appModel) openCommandPalette() tea.Cmd {
    return util.CmdHandler(dialogs.OpenDialogMsg{
        Model: commands.NewCommandDialog(a.selectedSessionID, a.pluginManager),
    })
}
```

### Custom Dialog Registration

```go
// DialogRegistry allows plugins to register custom dialogs
type DialogRegistry struct {
    factories map[string]DialogFactory
}

type DialogFactory interface {
    CreateDialog(ctx PluginContext, args map[string]string) (util.Model, error)
    DialogID() string
}

func (dr *DialogRegistry) RegisterDialog(factory DialogFactory) {
    dr.factories[factory.DialogID()] = factory
}

func (dr *DialogRegistry) CreateDialog(dialogID string, ctx PluginContext, args map[string]string) (util.Model, error) {
    if factory, ok := dr.factories[dialogID]; ok {
        return factory.CreateDialog(ctx, args)
    }
    return nil, fmt.Errorf("unknown dialog: %s", dialogID)
}
```

## Sidebar Plugin Architecture

### Sidebar Section Management

```go
// SidebarManager handles sidebar plugin integration
type SidebarManager struct {
    sections []SidebarPlugin
    registry map[string]SidebarPlugin
}

func (sm *SidebarManager) Register(plugin SidebarPlugin) {
    sm.sections = append(sm.sections, plugin)
    sm.registry[plugin.Info().ID] = plugin
    
    // Sort by priority
    sort.Slice(sm.sections, func(i, j int) bool {
        return sm.getSectionPriority(sm.sections[i]) > sm.getSectionPriority(sm.sections[j])
    })
}

func (sm *SidebarManager) GetSections(ctx PluginContext) []SidebarSection {
    var sections []SidebarSection
    
    for _, plugin := range sm.sections {
        // Skip plugins that don't support current mode
        if ctx.CompactMode && !plugin.SupportsCompactMode() {
            continue
        }
        
        if section, err := plugin.CreateSection(ctx); err == nil {
            sections = append(sections, section)
        }
    }
    
    return sections
}

// Integration with existing sidebar
// In internal/tui/components/chat/sidebar/sidebar.go:

func (m *sidebarCmp) View() string {
    t := styles.CurrentTheme()
    parts := []string{}
    
    // Existing logo, title, cwd, model blocks...
    parts = append(parts, m.currentModelBlock())
    
    // Add plugin sections
    if pluginManager := GetGlobalPluginManager(); pluginManager != nil {
        ctx := PluginContext{
            SessionID:   m.session.ID,
            Session:     m.session,
            Width:       m.width,
            Height:      m.height,
            CompactMode: m.compactMode,
            Theme:       getCurrentTheme(),
        }
        
        pluginSections := pluginManager.sidebarManager.GetSections(ctx)
        for _, section := range pluginSections {
            if !m.compactMode {
                parts = append(parts, "", section.View())
            } else {
                // Handle compact mode rendering
                parts = append(parts, m.renderSectionCompact(section))
            }
        }
    }
    
    // Existing files, LSP, MCP blocks...
    if m.session.ID != "" {
        parts = append(parts, "", m.filesBlock())
    }
    parts = append(parts, "", m.lspBlock(), "", m.mcpBlock())
    
    return m.renderWithStyle(parts)
}
```

### Dynamic Section Sizing

```go
// SectionLayoutManager handles dynamic sizing for plugin sections
type SectionLayoutManager struct {
    builtinSections int // Number of built-in sections
    pluginSections  []SidebarSection
    totalHeight     int
    compactMode     bool
}

func (slm *SectionLayoutManager) CalculateLayout() map[string]int {
    availableHeight := slm.calculateAvailableHeight()
    
    // Reserve space for built-in sections
    builtinSpace := slm.calculateBuiltinSpace()
    pluginSpace := availableHeight - builtinSpace
    
    if pluginSpace <= 0 {
        return map[string]int{} // No space for plugins
    }
    
    // Distribute space among plugin sections based on priority
    sectionHeights := make(map[string]int)
    totalPriority := 0
    
    for _, section := range slm.pluginSections {
        totalPriority += section.Priority()
    }
    
    for _, section := range slm.pluginSections {
        sectionID := section.Title()
        priority := section.Priority()
        allocatedHeight := (pluginSpace * priority) / totalPriority
        sectionHeights[sectionID] = max(2, allocatedHeight) // Minimum 2 lines
    }
    
    return sectionHeights
}
```

## Security and Isolation

### Plugin Sandboxing

```go
// PluginSandbox provides security isolation for UI plugins
type PluginSandbox struct {
    allowedPaths   []string
    blockedSymbols []string
    resourceLimits ResourceLimits
}

type ResourceLimits struct {
    MaxMemoryMB     int
    MaxGoroutines   int
    MaxFileHandles  int
    RenderTimeout   time.Duration
}

func (ps *PluginSandbox) ValidatePlugin(plugin UIPlugin) error {
    info := plugin.Info()
    
    // Validate plugin metadata
    if err := ps.validatePluginInfo(info); err != nil {
        return err
    }
    
    // Check plugin capabilities
    for _, capability := range info.Capabilities {
        if !ps.isCapabilityAllowed(capability) {
            return fmt.Errorf("capability %s not allowed", capability)
        }
    }
    
    return nil
}

func (ps *PluginSandbox) WrapPlugin(plugin UIPlugin) UIPlugin {
    return &sandboxedPlugin{
        plugin: plugin,
        limits: ps.resourceLimits,
    }
}

type sandboxedPlugin struct {
    plugin UIPlugin
    limits ResourceLimits
}

func (sp *sandboxedPlugin) Render(ctx PluginContext, msg message.Message) (util.Model, error) {
    // Set rendering timeout
    renderCtx, cancel := context.WithTimeout(context.Background(), sp.limits.RenderTimeout)
    defer cancel()
    
    // Monitor resource usage
    monitor := NewResourceMonitor(sp.limits)
    monitor.Start()
    defer monitor.Stop()
    
    // Execute plugin render with monitoring
    result := make(chan renderResult, 1)
    go func() {
        component, err := sp.plugin.(ConversationWidget).Render(ctx, msg)
        result <- renderResult{component, err}
    }()
    
    select {
    case res := <-result:
        if monitor.ExceededLimits() {
            return nil, fmt.Errorf("plugin exceeded resource limits")
        }
        return res.component, res.err
        
    case <-renderCtx.Done():
        return nil, fmt.Errorf("plugin render timeout")
    }
}
```

### Configuration Validation

```go
// PluginConfigValidator ensures safe plugin configuration
type PluginConfigValidator struct {
    allowedKeys    map[string]bool
    typeValidators map[string]func(interface{}) bool
}

func (pcv *PluginConfigValidator) ValidateConfig(pluginID string, config map[string]interface{}) error {
    for key, value := range config {
        // Check if key is allowed for this plugin
        if !pcv.isKeyAllowed(pluginID, key) {
            return fmt.Errorf("configuration key %s not allowed for plugin %s", key, pluginID)
        }
        
        // Validate value type
        if validator, ok := pcv.typeValidators[key]; ok {
            if !validator(value) {
                return fmt.Errorf("invalid value type for key %s", key)
            }
        }
        
        // Check for dangerous values
        if pcv.isDangerousValue(value) {
            return fmt.Errorf("potentially dangerous configuration value: %v", value)
        }
    }
    
    return nil
}
```

## Implementation Roadmap

### Phase 1: Core Infrastructure (1-2 weeks)
1. **Plugin Interface Definition** (2-3 days)
   - Define core UI plugin interfaces
   - Create plugin context and metadata structures
   - Implement basic plugin registry

2. **Conversation Widget System** (3-4 days)
   - Extend message rendering system for plugins
   - Create widget registration and discovery
   - Integrate with existing message components

3. **Plugin Loading Infrastructure** (2-3 days)
   - Implement Go plugin loading system
   - Create plugin discovery and validation
   - Add basic error handling and fallbacks

### Phase 2: Menu and Command Extensions (1-2 weeks)
1. **Command System Integration** (3-4 days)
   - Extend command palette for plugin commands
   - Add custom keybinding support
   - Implement command argument handling

2. **Dialog System Extensions** (2-3 days)
   - Add custom dialog registration
   - Extend dialog manager for plugin dialogs
   - Create dialog factory system

3. **Navigation Integration** (1-2 days)
   - Integrate plugin commands with global keymap
   - Add menu category support
   - Implement command priority system

### Phase 3: Sidebar Plugin System (1-2 weeks)
1. **Sidebar Extension Framework** (3-4 days)
   - Create sidebar section interface
   - Implement dynamic section management
   - Add responsive layout support

2. **Layout Management** (2-3 days)
   - Dynamic section sizing algorithms
   - Compact mode support for plugins
   - Section priority and ordering

3. **Real-time Data Integration** (1-2 days)
   - Connect plugins to pub/sub system
   - Session context management
   - Live data updates

### Phase 4: Security and Reliability (1 week)
1. **Security Implementation** (3-4 days)
   - Plugin sandboxing system
   - Resource monitoring and limits
   - Configuration validation

2. **Error Handling and Recovery** (2-3 days)
   - Plugin failure isolation
   - Graceful degradation
   - Error reporting and logging

### Phase 5: Development Tools and Documentation (1 week)
1. **Development Support** (2-3 days)
   - Hot reloading for development
   - Plugin debugging tools
   - Development templates

2. **Documentation and Examples** (2-3 days)
   - Plugin development guide
   - API reference documentation
   - Example plugin implementations

## Example Plugin Implementations

### Chat Visualization Plugin

```go
// Example: Chart rendering plugin for displaying data visualizations
package main

import (
    "encoding/json"
    "github.com/charmbracelet/crush/internal/tui/uiplugins"
    "github.com/charmbracelet/crush/internal/message"
)

type ChartWidget struct {
    chartTypes map[string]ChartRenderer
}

func (cw *ChartWidget) Info() uiplugins.WidgetInfo {
    return uiplugins.WidgetInfo{
        Name:        "Chart Visualizer",
        Description: "Renders charts and graphs from structured data",
        MessageTypes: []message.MessageRole{message.Assistant, message.Tool},
        ToolNames:   []string{"data_analysis", "chart_generator"},
    }
}

func (cw *ChartWidget) CanRender(msg message.Message) bool {
    // Check if message contains chart data
    content := msg.Content().Text
    return strings.Contains(content, "```chart") || 
           strings.Contains(content, "```graph") ||
           (len(msg.ToolCalls()) > 0 && cw.hasChartTool(msg.ToolCalls()))
}

func (cw *ChartWidget) Render(ctx uiplugins.PluginContext, msg message.Message) (util.Model, error) {
    chartData, err := cw.extractChartData(msg)
    if err != nil {
        return nil, err
    }
    
    renderer := cw.getRenderer(chartData.Type)
    return renderer.CreateChart(ctx, chartData)
}

func (cw *ChartWidget) Priority() int {
    return 100 // High priority for chart data
}

// Plugin entry point
func NewUIPlugin() (uiplugins.UIPlugin, error) {
    return &ChartWidget{
        chartTypes: make(map[string]ChartRenderer),
    }, nil
}
```

### System Monitor Sidebar Plugin

```go
// Example: System monitoring sidebar plugin
type SystemMonitorPlugin struct {
    metrics *SystemMetrics
}

func (smp *SystemMonitorPlugin) Info() uiplugins.PluginInfo {
    return uiplugins.PluginInfo{
        ID:          "system_monitor",
        Name:        "System Monitor",
        Description: "Real-time system resource monitoring",
        Capabilities: []uiplugins.PluginCapability{
            uiplugins.CapabilitySidebarSection,
        },
    }
}

func (smp *SystemMonitorPlugin) CreateSection(ctx uiplugins.PluginContext) (uiplugins.SidebarSection, error) {
    return &systemMonitorSection{
        metrics: smp.metrics,
        width:   ctx.Width,
        height:  ctx.Height,
    }, nil
}

func (smp *SystemMonitorPlugin) SupportsCompactMode() bool {
    return true
}

type systemMonitorSection struct {
    metrics *SystemMetrics
    width   int
    height  int
}

func (sms *systemMonitorSection) View() string {
    stats := sms.metrics.GetCurrent()
    
    items := []string{
        fmt.Sprintf("CPU: %d%%", stats.CPUPercent),
        fmt.Sprintf("Memory: %s / %s", formatBytes(stats.MemoryUsed), formatBytes(stats.MemoryTotal)),
        fmt.Sprintf("Disk: %d%%", stats.DiskPercent),
    }
    
    return lipgloss.JoinVertical(lipgloss.Left, items...)
}

func (sms *systemMonitorSection) Title() string {
    return "System"
}

func (sms *systemMonitorSection) Priority() int {
    return 50 // Medium priority
}
```

### Command Extension Plugin

```go
// Example: Custom development commands plugin
type DevCommandsPlugin struct {
    projectRoot string
}

func (dcp *DevCommandsPlugin) Commands(ctx uiplugins.PluginContext) []uiplugins.Command {
    return []uiplugins.Command{
        {
            ID:          "dev.run_tests",
            Name:        "Run Tests",
            Description: "Execute project test suite",
            Category:    "Development",
            KeyBinding:  "ctrl+t",
        },
        {
            ID:          "dev.build_project", 
            Name:        "Build Project",
            Description: "Build the current project",
            Category:    "Development",
            Args: []uiplugins.CommandArg{
                {
                    Name:        "target",
                    Description: "Build target (debug/release)",
                    Type:        uiplugins.ArgTypeString,
                    Default:     "debug",
                },
            },
        },
    }
}

func (dcp *DevCommandsPlugin) ExecuteCommand(ctx uiplugins.PluginContext, commandID string, args map[string]string) tea.Cmd {
    switch commandID {
    case "dev.run_tests":
        return dcp.runTests(ctx)
    case "dev.build_project":
        target := args["target"]
        return dcp.buildProject(ctx, target)
    }
    return nil
}
```

## Conclusion

This research provides a comprehensive foundation for implementing a UI plugin system in the Crush coding agent. The design focuses on:

1. **Extensibility**: Clean interfaces for conversation widgets, menu commands, and sidebar visualizations
2. **Integration**: Seamless integration with existing Bubble Tea architecture
3. **Flexibility**: Support for multiple plugin types and custom behaviors
4. **Security**: Proper sandboxing and resource management for plugin execution
5. **Developer Experience**: Hot reloading, debugging tools, and comprehensive documentation

The plugin system will enable powerful UI extensibility for custom visualizations, enhanced workflows, and new interaction modalities while maintaining the performance and reliability of the core TUI system.

Key advantages:
- **Hot-pluggable**: Plugins can be loaded/unloaded without restarting
- **Type-safe**: Go plugin system provides compile-time safety
- **Resource-managed**: Built-in protection against resource exhaustion  
- **Developer-friendly**: Clear APIs and development tools

Next steps involve implementing the core infrastructure, followed by menu extensions, sidebar plugins, security measures, and comprehensive documentation with example implementations.
