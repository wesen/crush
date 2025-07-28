#!/bin/bash

# Build the test hook plugin as a shared library
echo "Building test hook plugin..."

# Clean any existing build
rm -f test_hook.so

# Build as plugin
go build -buildmode=plugin -o test_hook.so main.go

if [ $? -eq 0 ]; then
    echo "✅ Test hook plugin built successfully: test_hook.so"
    ls -la test_hook.so
else
    echo "❌ Failed to build test hook plugin"
    exit 1
fi
