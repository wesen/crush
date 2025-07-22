package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/charmbracelet/crush/internal/db"
	"github.com/charmbracelet/crush/internal/message"
	"github.com/charmbracelet/crush/internal/session"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// SessionWithChildren represents a session with its nested children
type SessionWithChildren struct {
	session.Session
	Children []SessionWithChildren `json:"children,omitempty" yaml:"children,omitempty"`
}

var sessionsCmd = &cobra.Command{
	Use:   "sessions",
	Short: "Manage sessions",
	Long:  `List and export sessions and their nested subsessions`,
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List sessions",
	Long:  `List all sessions in a hierarchical format`,
	RunE: func(cmd *cobra.Command, args []string) error {
		format, _ := cmd.Flags().GetString("format")
		return runSessionsList(cmd.Context(), format)
	},
}

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export sessions",
	Long:  `Export all sessions and their nested subsessions to different formats`,
	RunE: func(cmd *cobra.Command, args []string) error {
		format, _ := cmd.Flags().GetString("format")
		return runSessionsExport(cmd.Context(), format)
	},
}

var exportConversationCmd = &cobra.Command{
	Use:   "export-conversation <session-id>",
	Short: "Export a single conversation",
	Long:  `Export a single session with all its messages as markdown for sharing`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sessionID := args[0]
		format, _ := cmd.Flags().GetString("format")
		return runExportConversation(cmd.Context(), sessionID, format)
	},
}

func init() {
	rootCmd.AddCommand(sessionsCmd)
	sessionsCmd.AddCommand(listCmd)
	sessionsCmd.AddCommand(exportCmd)
	sessionsCmd.AddCommand(exportConversationCmd)

	listCmd.Flags().StringP("format", "f", "text", "Output format (text, json, yaml, markdown)")
	exportCmd.Flags().StringP("format", "f", "json", "Export format (json, yaml, markdown)")
	exportConversationCmd.Flags().StringP("format", "f", "markdown", "Export format (markdown, json, yaml)")
}

func runSessionsList(ctx context.Context, format string) error {
	sessionService, err := createSessionService(ctx)
	if err != nil {
		return err
	}

	sessions, err := buildSessionTree(ctx, sessionService)
	if err != nil {
		return err
	}

	return formatOutput(sessions, format, false)
}

func runSessionsExport(ctx context.Context, format string) error {
	sessionService, err := createSessionService(ctx)
	if err != nil {
		return err
	}

	sessions, err := buildSessionTree(ctx, sessionService)
	if err != nil {
		return err
	}

	return formatOutput(sessions, format, true)
}

func runExportConversation(ctx context.Context, sessionID, format string) error {
	sessionService, messageService, err := createServices(ctx)
	if err != nil {
		return err
	}

	// Get the session
	sess, err := sessionService.Get(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to get session %s: %w", sessionID, err)
	}

	// Get all messages for the session
	messages, err := messageService.List(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to get messages for session %s: %w", sessionID, err)
	}

	return formatConversation(sess, messages, format)
}

func createSessionService(ctx context.Context) (session.Service, error) {
	cwd, err := getCwd()
	if err != nil {
		return nil, err
	}

	cfg, err := config.Init(cwd, false)
	if err != nil {
		return nil, err
	}

	conn, err := db.Connect(ctx, cfg.Options.DataDirectory)
	if err != nil {
		return nil, err
	}

	queries := db.New(conn)
	return session.NewService(queries), nil
}

func createServices(ctx context.Context) (session.Service, message.Service, error) {
	cwd, err := getCwd()
	if err != nil {
		return nil, nil, err
	}

	cfg, err := config.Init(cwd, false)
	if err != nil {
		return nil, nil, err
	}

	conn, err := db.Connect(ctx, cfg.Options.DataDirectory)
	if err != nil {
		return nil, nil, err
	}

	queries := db.New(conn)
	sessionService := session.NewService(queries)
	messageService := message.NewService(queries)
	return sessionService, messageService, nil
}

func getCwd() (string, error) {
	// This could be enhanced to use the same logic as root.go
	cwd, err := getCwdFromFlags()
	if err != nil {
		return "", err
	}
	return cwd, nil
}

func getCwdFromFlags() (string, error) {
	return os.Getwd()
}

func buildSessionTree(ctx context.Context, sessionService session.Service) ([]SessionWithChildren, error) {
	// Get all top-level sessions (no parent)
	topLevelSessions, err := sessionService.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}

	var result []SessionWithChildren
	for _, sess := range topLevelSessions {
		sessionWithChildren, err := buildSessionWithChildren(ctx, sessionService, sess)
		if err != nil {
			return nil, err
		}
		result = append(result, sessionWithChildren)
	}

	return result, nil
}

func buildSessionWithChildren(ctx context.Context, sessionService session.Service, sess session.Session) (SessionWithChildren, error) {
	children, err := sessionService.ListChildren(ctx, sess.ID)
	if err != nil {
		return SessionWithChildren{}, fmt.Errorf("failed to list children for session %s: %w", sess.ID, err)
	}

	var childrenWithChildren []SessionWithChildren
	for _, child := range children {
		childWithChildren, err := buildSessionWithChildren(ctx, sessionService, child)
		if err != nil {
			return SessionWithChildren{}, err
		}
		childrenWithChildren = append(childrenWithChildren, childWithChildren)
	}

	return SessionWithChildren{
		Session:  sess,
		Children: childrenWithChildren,
	}, nil
}

func formatOutput(sessions []SessionWithChildren, format string, includeMetadata bool) error {
	switch strings.ToLower(format) {
	case "json":
		return formatJSON(sessions)
	case "yaml":
		return formatYAML(sessions)
	case "markdown", "md":
		return formatMarkdown(sessions, includeMetadata)
	case "text":
		return formatText(sessions)
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}
}

func formatJSON(sessions []SessionWithChildren) error {
	data, err := json.MarshalIndent(sessions, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

func formatYAML(sessions []SessionWithChildren) error {
	data, err := yaml.Marshal(sessions)
	if err != nil {
		return fmt.Errorf("failed to marshal YAML: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

func formatMarkdown(sessions []SessionWithChildren, includeMetadata bool) error {
	fmt.Println("# Sessions")
	fmt.Println()

	if len(sessions) == 0 {
		fmt.Println("No sessions found.")
		return nil
	}

	for _, sess := range sessions {
		printSessionMarkdown(sess, 0, includeMetadata)
	}

	return nil
}

func formatText(sessions []SessionWithChildren) error {
	if len(sessions) == 0 {
		fmt.Println("No sessions found.")
		return nil
	}

	for _, sess := range sessions {
		printSessionText(sess, 0)
	}

	return nil
}

func printSessionMarkdown(sess SessionWithChildren, level int, includeMetadata bool) {
	indent := strings.Repeat("#", level+2)
	fmt.Printf("%s %s\n", indent, sess.Title)
	fmt.Println()

	if includeMetadata {
		fmt.Printf("- **ID**: %s\n", sess.ID)
		if sess.ParentSessionID != "" {
			fmt.Printf("- **Parent**: %s\n", sess.ParentSessionID)
		}
		fmt.Printf("- **Messages**: %d\n", sess.MessageCount)
		fmt.Printf("- **Tokens**: %d prompt, %d completion\n", sess.PromptTokens, sess.CompletionTokens)
		fmt.Printf("- **Cost**: $%.4f\n", sess.Cost)
		fmt.Printf("- **Created**: %s\n", formatTimestamp(sess.CreatedAt))
		fmt.Printf("- **Updated**: %s\n", formatTimestamp(sess.UpdatedAt))
		fmt.Println()
	}

	for _, child := range sess.Children {
		printSessionMarkdown(child, level+1, includeMetadata)
	}
}

func printSessionText(sess SessionWithChildren, level int) {
	indent := strings.Repeat("  ", level)
	fmt.Printf("%s• %s (ID: %s, Messages: %d, Cost: $%.4f)\n",
		indent, sess.Title, sess.ID, sess.MessageCount, sess.Cost)

	for _, child := range sess.Children {
		printSessionText(child, level+1)
	}
}

func formatTimestamp(timestamp int64) string {
	// Assuming timestamp is Unix seconds
	return time.Unix(timestamp, 0).Format("2006-01-02 15:04:05")
}

func formatConversation(sess session.Session, messages []message.Message, format string) error {
	switch strings.ToLower(format) {
	case "markdown", "md":
		return formatConversationMarkdown(sess, messages)
	case "json":
		return formatConversationJSON(sess, messages)
	case "yaml":
		return formatConversationYAML(sess, messages)
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}
}

func formatConversationMarkdown(sess session.Session, messages []message.Message) error {
	fmt.Printf("# %s\n\n", sess.Title)

	// Session metadata
	fmt.Printf("**Session ID:** %s  \n", sess.ID)
	fmt.Printf("**Created:** %s  \n", formatTimestamp(sess.CreatedAt))
	fmt.Printf("**Messages:** %d  \n", sess.MessageCount)
	fmt.Printf("**Tokens:** %d prompt, %d completion  \n", sess.PromptTokens, sess.CompletionTokens)
	if sess.Cost > 0 {
		fmt.Printf("**Cost:** $%.4f  \n", sess.Cost)
	}
	fmt.Println()
	fmt.Println("---")
	fmt.Println()

	for i, msg := range messages {
		formatMessageMarkdown(msg, i+1)
	}

	return nil
}

func formatMessageMarkdown(msg message.Message, index int) {
	// Role header
	switch msg.Role {
	case message.User:
		fmt.Printf("## 👤 User\n\n")
	case message.Assistant:
		fmt.Printf("## 🤖 Assistant")
		if msg.Model != "" {
			fmt.Printf(" (%s)", msg.Model)
		}
		fmt.Printf("\n\n")
	case message.System:
		fmt.Printf("## ⚙️ System\n\n")
	case message.Tool:
		fmt.Printf("## 🔧 Tool\n\n")
	}

	// Process each part
	for _, part := range msg.Parts {
		switch p := part.(type) {
		case message.TextContent:
			fmt.Printf("%s\n\n", p.Text)
		case message.ReasoningContent:
			if p.Thinking != "" {
				fmt.Printf("### 🧠 Reasoning\n\n")
				fmt.Printf("```\n%s\n```\n\n", p.Thinking)
			}
		case message.ToolCall:
			fmt.Printf("### 🔧 Tool Call: %s\n\n", p.Name)
			fmt.Printf("**ID:** %s  \n", p.ID)
			if p.Input != "" {
				fmt.Printf("**Input:**\n```json\n%s\n```\n\n", p.Input)
			}
		case message.ToolResult:
			fmt.Printf("### 📝 Tool Result: %s\n\n", p.Name)
			if p.IsError {
				fmt.Printf("**❌ Error:**\n```\n%s\n```\n\n", p.Content)
			} else {
				fmt.Printf("**✅ Result:**\n```\n%s\n```\n\n", p.Content)
			}
		case message.ImageURLContent:
			fmt.Printf("![Image](%s)\n\n", p.URL)
		case message.BinaryContent:
			fmt.Printf("**File:** %s (%s)\n\n", p.Path, p.MIMEType)
		case message.Finish:
			if p.Reason != message.FinishReasonEndTurn {
				fmt.Printf("**Finish Reason:** %s\n", p.Reason)
				if p.Message != "" {
					fmt.Printf("**Message:** %s\n", p.Message)
				}
				fmt.Println()
			}
		}
	}

	fmt.Println("---")
	fmt.Println()
}

func formatConversationJSON(sess session.Session, messages []message.Message) error {
	data := struct {
		Session  session.Session   `json:"session"`
		Messages []message.Message `json:"messages"`
	}{
		Session:  sess,
		Messages: messages,
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	fmt.Println(string(jsonData))
	return nil
}

func formatConversationYAML(sess session.Session, messages []message.Message) error {
	data := struct {
		Session  session.Session   `yaml:"session"`
		Messages []message.Message `yaml:"messages"`
	}{
		Session:  sess,
		Messages: messages,
	}

	yamlData, err := yaml.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal YAML: %w", err)
	}
	fmt.Println(string(yamlData))
	return nil
}
