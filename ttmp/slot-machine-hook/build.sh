#!/bin/bash

# Build the slot machine hook plugin
# This script compiles the Go plugin as a shared library (.so file)

set -e

echo "🎰 Building Slot Machine Hook Plugin..."

# Set plugin name
PLUGIN_NAME="slot_machine_hook"

# Clean up any existing plugin
if [ -f "${PLUGIN_NAME}.so" ]; then
    echo "Removing existing plugin: ${PLUGIN_NAME}.so"
    rm "${PLUGIN_NAME}.so"
fi

# Build the plugin
echo "Compiling plugin..."
go build -buildmode=plugin -o "${PLUGIN_NAME}.so" main.go

# Verify the plugin was built
if [ -f "${PLUGIN_NAME}.so" ]; then
    echo "✅ Plugin built successfully: ${PLUGIN_NAME}.so"
    ls -la "${PLUGIN_NAME}.so"
else
    echo "❌ Plugin build failed!"
    exit 1
fi

echo ""
echo "🎰 Slot Machine Hook Plugin is ready!"
echo ""
echo "To use this plugin:"
echo "1. Copy ${PLUGIN_NAME}.so to a directory configured in your crush config"
echo "2. Add plugin configuration to your crush.json:"
echo '   {
     "hooks": {
       "enabled": true,
       "plugin_dirs": ["./ttmp/slot-machine-hook"],
       "plugins": {
         "slot_machine_hook": {
           "enabled": true,
           "config": {
             "win_amounts": [1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500]
           }
         }
       }
     }
   }'
echo ""
echo "3. Run crush and watch for slot machine winnings after tool executions! 🎰"
