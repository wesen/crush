# Test Hook Plugin

This is a simple test plugin that demonstrates the Crush hook plugin system.

## Building

To build the plugin as a shared library:

```bash
./build.sh
```

This will create `test_hook.so` which can be loaded by Crush.

## Configuration

Add the plugin to your `crush.json` configuration:

```json
{
  "hooks": {
    "plugin_dirs": ["./examples/test_hook"],
    "plugins": {
      "test_hook": {
        "path": "./examples/test_hook/test_hook.so",
        "config": {
          "message_prefix": "Plugin: "
        },
        "disabled": false
      }
    }
  }
}
```

## Testing

1. Build the plugin: `./build.sh`
2. Set environment variable: `export CRUSH_USE_TEST_PLUGIN_HOOK=1`
3. Run Crush with the plugin configuration
4. Execute any tool - the plugin will inject a test message

## How It Works

The plugin implements the `TransformToolHook` interface, which allows it to:

- Intercept tool execution results
- Access the full session context
- Inject new messages
- Modify tool results (if needed)

The plugin demonstrates:

- Hook metadata (`Info()` method)
- Service initialization (`Initialize()` method)
- Transform tool hook implementation (`TransformAfterTool()` method)
- Message injection using the message service
- Environment-based activation
