package main

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/charmbracelet/crush/hooks"
)

// SlotMachineHook injects fun slot machine winnings after tool executions
type SlotMachineHook struct {
	messageService hooks.MessageService
	config         map[string]interface{}
	enabled        bool
	winAmounts     []int
}

// Info returns metadata about this hook
func (s *SlotMachineHook) Info() hooks.HookInfo {
	return hooks.HookInfo{
		Name:        "slot_machine_hook",
		Version:     "1.0.0",
		Description: "🎰 Adds random slot machine winnings after tool executions for fun!",
	}
}

// Initialize sets up the hook with configuration and services
func (s *SlotMachineHook) Initialize(config map[string]interface{}, services hooks.ServiceRegistry) error {
	s.config = config
	s.messageService = services.MessageService()
	s.enabled = true

	// Default win amounts
	s.winAmounts = []int{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500}

	// Override with config if provided
	if config != nil {
		if amounts, ok := config["win_amounts"].([]interface{}); ok {
			s.winAmounts = make([]int, len(amounts))
			for i, amount := range amounts {
				if val, ok := amount.(float64); ok {
					s.winAmounts[i] = int(val)
				}
			}
		}
	}
	
	return nil
}

// TransformSession implements TransformSessionHook to get session-level access
func (s *SlotMachineHook) TransformSession(ctx context.Context, sess *hooks.Session) (hooks.TransformResult[hooks.Session], error) {
	// This hook doesn't modify the session directly, but needs access for message injection
	return hooks.NoChange[hooks.Session](), nil
}

// TransformAfterTool implements TransformToolHook for the trigger event
func (s *SlotMachineHook) TransformAfterTool(ctx context.Context, resultCtx *hooks.ToolTransformContext) (hooks.TransformResult[hooks.ToolTransformContext], error) {
	if !s.enabled {
		return hooks.NoChange[hooks.ToolTransformContext](), nil
	}
	
	// Skip if tool execution failed
	if resultCtx.Err != nil {
		return hooks.NoChange[hooks.ToolTransformContext](), nil
	}
	
	// Generate random winning amount
	winAmount := s.winAmounts[rand.Intn(len(s.winAmounts))]
	
	// Create fun slot machine message
	slotEmojis := []string{"🍒", "🍋", "🍊", "🍇", "⭐", "💎", "🔔", "💰"}
	emoji1 := slotEmojis[rand.Intn(len(slotEmojis))]
	emoji2 := slotEmojis[rand.Intn(len(slotEmojis))]
	emoji3 := slotEmojis[rand.Intn(len(slotEmojis))]
	
	var message string
	if winAmount >= 1000 {
		message = fmt.Sprintf("🎰 JACKPOT! %s%s%s You won $%d at the slot machine! 💰💰💰", emoji1, emoji2, emoji3, winAmount)
	} else if winAmount >= 100 {
		message = fmt.Sprintf("🎰 Big win! %s%s%s You won $%d at the slot machine! 🎉", emoji1, emoji2, emoji3, winAmount)
	} else {
		message = fmt.Sprintf("🎰 %s%s%s You won $%d at the slot machine! 🎲", emoji1, emoji2, emoji3, winAmount)
	}
	
	// Inject the message into the session
	_, err := s.messageService.Create(ctx, resultCtx.SessionID, hooks.CreateMessageParams{
		Role: hooks.System,
		Parts: []hooks.ContentPart{
			hooks.TextContent{Text: message},
		},
		Model:    "hook-system",
		Provider: "slot-machine-hook",
	})
	
	if err != nil {
		return hooks.TransformResult[hooks.ToolTransformContext]{}, fmt.Errorf("failed to inject slot machine message: %w", err)
	}

	// Return original context unchanged (non-destructive)
	return hooks.NoChange[hooks.ToolTransformContext](), nil
}

// NewHook is the plugin entry point that the plugin loader will call
func NewHook() hooks.Hook {
	// Seed random number generator
	rand.Seed(time.Now().UnixNano())
	
	return &SlotMachineHook{}
}

// Ensure we implement the correct interfaces
var _ hooks.HookInitializer = (*SlotMachineHook)(nil)
var _ hooks.TransformSessionHook = (*SlotMachineHook)(nil)
var _ hooks.TransformToolHook = (*SlotMachineHook)(nil)
