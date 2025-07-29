# 🎰 Slot Machine Hook Plugin

A fun Crush hook plugin that adds random slot machine winnings after tool executions!

## What it does

This plugin injects amusing slot machine messages into your conversations after tool executions. You'll see messages like:

- `🎰 🍒🍋🍊 You won $25 at the slot machine! 🎲`
- `🎰 Big win! ⭐💎🔔 You won $500 at the slot machine! 🎉`
- `🎰 JACKPOT! 💰💰💰 You won $2500 at the slot machine! 💰💰💰`

## Building the Plugin

```bash
cd ttmp/slot-machine-hook
./build.sh
```

This will create `slot_machine_hook.so` in the current directory.

## Configuration

Add to your `crush.json`:

```json
{
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
}
```

## Usage

1. Build the plugin with `./build.sh`
2. Configure crush to load the plugin
3. Run crush and execute any tool command
4. Watch for slot machine winnings! 🎰

## Configuration Options

- `enabled`: Enable/disable the plugin (default: true)
- `win_amounts`: Array of possible winning amounts in dollars (default: [1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500])

## How it Works

The plugin implements the `AfterToolResulter` interface and:

1. Triggers after every successful tool execution
2. Generates a random winning amount from the configured array
3. Creates a fun message with slot machine emojis
4. Injects the message into the session as a system message
5. Logs the activity for debugging

The plugin uses the service registry to access the MessageService for injecting messages into the conversation.
