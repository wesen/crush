Read file: crush/ttmp/2025-07-31/01-current-hook-system-overview-o4.md
Read file: crush/hooks/hooks.go
Read file: crush/internal/hooks/manager.go
Read file: crush/internal/hooks/transform_manager.go
Read file: crush/internal/hooks/circuit_breaker.go
Read file: crush/internal/hooks/plugin_loader.go
Read file: crush/internal/hooks/plugin_manager.go
Read file: crush/internal/hooks/registry.go
Read file: crush/internal/hooks/builtin/logging/logging.go
Read file: crush/internal/hooks/builtin/magicmessage/magicmessage.go

Ran tool

Read file: crush/ttmp/2025-07-31/01-current-hook-system-overview.md

Here’s a consolidated report on the Crush hook system, based on the code in `crush/internal/hooks/` and the “Current Implementation Analysis” in  
`crush/ttmp/2025-07-31/01-current-hook-system-overview.md`. I’ll describe each component, how they’re wired together, and call out the critical gaps identified in the overview.

1. Core Interfaces (`internal/hooks/hooks.go`)  
   • HookInfo / Hook  
     – Every hook must implement `Info() HookInfo` to provide its name, version, and description.  
   • Event-specific interfaces  
     – `BeforeLLMCaller` / `AfterLLMInferencer` / `BeforeToolCaller` / `AfterToolResulter`  
       allow hooks to observe LLM calls and tool calls.  
   • Transform interfaces  
     – `TransformToolHook` / `TransformSessionHook` let hooks mutate tool-result context or session state.  
   • ServiceRegistry  
     – Exposes `MessageService()` and `SessionService()` for hook initialization (see gap: no `NewServiceRegistry` in `registry.go`).  

2. Event Manager (`internal/hooks/manager.go`)  
   • Creation  
     – `New()` and `NewWithCircuitBreakerConfig(...)` build a `Manager` with its own `CircuitBreakerManager`.  
   • Registration  
     – `Add(h Hook)` logs and appends each hook.  
   • Emission  
     – `EmitBeforeLLM(ctx, *LLMCallCtx)`, `EmitAfterLLM(ctx, *LLMRespCtx)`, `EmitBeforeTool(ctx, *ToolCallCtx)`,  
       `EmitAfterTool(ctx, *ToolResCtx)` iterate over registered hooks, select those implementing the marker  
       interface, and invoke them via `safeCall(...)`.  
   • Circuit Breaker  
     – Before each hook invocation, `safeCall` routes execution through the circuit breaker obtained from  
       `m.circuitBreakerManager.GetCircuitBreaker(hookName)`.  

3. Transform Manager (`internal/hooks/transform_manager.go`)  
   • Creation  
     – `NewTransformManager(sr ServiceRegistry)` builds with default CB config.  
   • Registration  
     – `Add(h Hook)` also calls `Initialize(config, services)` if the hook implements `HookInitializer`.  
   • Execution  
     – `ExecuteTransformAfterTool(ctx, *ToolTransformContext)` scans for `TransformToolHook` hooks,  
       calls each via `safeTransformCall`, and applies any `Modified` or `Skip` flags and merges metadata.  

4. Circuit Breaker System (`internal/hooks/circuit_breaker.go`)  
   • Patterns  
     – Implements Closed/Open/HalfOpen with thresholds, timeouts, per-hook config, state-transition logging.  
   • Manager  
     – `CircuitBreakerManager` lazily creates breakers per hook name (`GetCircuitBreaker`) using default  
       or per-hook `CircuitBreakerConfig`.  
   • Metrics & Health  
     – Provides `Metrics()`, `GetHealthStatus()`, `ResetAll()`, `ResetHook(name)` for introspection and control.  

5. Plugin System  
   A. Loader (`internal/hooks/plugin_loader.go`)  
      – `DiscoverPlugins([]dirs)`: finds `.so` files  
      – `LoadPlugin(name,path,config)`: uses Go’s `plugin.Open`, looks up `NewHook() Hook`, calls it, and records metadata.  
      – `LoadPluginsFromConfig(pluginDirs, map[string]interface{}) ([]Hook,[]LoadError)` merges discovered and  
        explicitly configured plugins.  
   B. Manager (`internal/hooks/plugin_manager.go`)  
      – `NewPluginManager(transformMgr, eventMgr, sr, logger)`  
      – `LoadPluginsFromConfig(...)` → calls loader, records load errors → loops through returned `[]Hook`  
        and calls `RegisterHook(h)`.  
      – `RegisterHook(h Hook)` inspects which interfaces `h` implements, calls `Initialize`, then:  
          • If transform hook → `transformMgr.Add(h)`  
          • If event hook → `eventMgr.Add(h)`  

6. Built-in Hooks (`internal/hooks/builtin/…`)  
   • LoggingHook (`logging/logging.go`)  
     – Implements all four event interfaces; writes structured JSON to `hooks.log`.  
   • MagicMessageHook (`magicmessage/magicmessage.go`)  
     – Implements `HookInitializer`, `TransformSessionHook` (no change), and `TransformToolHook` in  
       `TransformAfterTool`; injects a random “magic word” message via `MessageService` when  
       `CRUSH_USE_MAGIC_MESSAGE_HOOK=1`.  

7. Public API (`crush/hooks/hooks.go`)  
   • Re-exports all core types, interfaces, convenience constructors, and common types (`Message`,  
     `Session`, `ToolCall`, etc.) from `internal/hooks`, `internal/message`, `internal/session`, and  
     `internal/llm/tools` for plugin authors.  

8. Integration Points (per overview)  
   • **App Init** (`internal/app/app.go:272–350`)  
     – Builds event and transform managers with app-level CB config.  
     – Instantiates `PluginManager` and calls `LoadPluginsFromConfig`.  
     – Creates agent via `agent.NewAgent(...)` but only passes the event manager and builds its own  
       transform manager internally (⚠️ missing app-level config).  
     – Registers all loaded hooks with the agent via `Agent.AddTransformHook` and built-ins via  
       `eventMgr.Add`.  
   • **Agent** (`internal/llm/agent/agent.go`)  
     – Exposes `EmitBeforeLLM`, `EmitAfterLLM`, `EmitBeforeTool`, `EmitAfterTool` at key points in  
       `streamAndHandleEvents`, `processEvent`, etc.  
     – **Gaps**:  
        1. Missing `NewServiceRegistry(msgSvc, sessSvc)` → compilation error when building service registry for hooks.  
        2. `EmitAfterLLM` only invoked for “completion” events, not every inference chunk.  
        3. Agent’s transform manager is always default-configured.  

9. Critical Gaps (from overview)  
   1. **Service Registry**: implement  
      ```go
      func NewServiceRegistry(msgSvc message.Service, sessSvc session.Service) ServiceRegistry { … }
      ```  
      in `internal/hooks/registry.go`.  
   2. **Agent Constructor**: extend `NewAgent(...)` signature in  
      `internal/llm/agent/agent.go` to accept a `*hooks.TransformManager` so the app-configured manager  
      (with CB settings) is used.  
   3. **App Wiring**: in `internal/app/app.go`, pass the configured transform manager into `agent.NewAgent`.  
   4. **Event Emission**: ensure `EmitAfterLLM` is called for all inference events, not just final completions.  
   5. **Plugin Config Shape**: align the map structure built in `app.go` with what `PluginLoader.LoadPluginsFromConfig` expects.  

—  
Overall, the individual pieces (managers, breakers, loader, built-ins) are solid and well-tested, but the integration layers between app, agent, and hooks have several omissions that prevent end-to-end functionality. Fixing the service registry, constructor signatures, wiring of the transform manager, and emission points should restore full hook-system operation.