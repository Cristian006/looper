package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/looper-ai/looper/pkg/agent"
	"github.com/looper-ai/looper/pkg/llm"
)

// ANSI color codes for terminal output
const (
	colorReset   = "\033[0m"
	colorBold    = "\033[1m"
	colorDim     = "\033[2m"
	colorCyan    = "\033[36m"
	colorGreen   = "\033[32m"
	colorYellow  = "\033[33m"
	colorBlue    = "\033[34m"
	colorRed     = "\033[31m"
	colorMagenta = "\033[35m"
)

var (
	version = "dev"
)

func init() {
	// Load .env file if it exists (silently ignore if not found)
	godotenv.Load()
}

func main() {
	// Define flags
	var (
		workspace        = flag.String("workspace", "", "Workspace directory path")
		provider         = flag.String("provider", "", "LLM provider (anthropic, openai)")
		model            = flag.String("model", "", "Model name (defaults to provider's default)")
		prompt           = flag.String("prompt", "", "Single prompt to execute (non-interactive mode)")
		systemPrompt     = flag.String("system", "", "Custom system prompt (overrides -system-prompt-id)")
		systemPromptID   = flag.String("system-prompt-id", "", "ID of prompt template to use as system prompt")
		promptsPath      = flag.String("prompts-path", "", "Path to prompts directory")
		maxIter          = flag.Int("max-iterations", 50, "Maximum tool call iterations")
		showVersion      = flag.Bool("version", false, "Show version")
		listSkills       = flag.Bool("list-skills", false, "List available skills and exit")
		listPrompts      = flag.Bool("list-prompts", false, "List available prompts and exit")
		disableBlacklist = flag.Bool("no-blacklist", false, "Disable command blacklist (dangerous)")
		blacklistFile    = flag.String("blacklist", "", "Path to custom blacklist file (one pattern per line)")
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Looper - AI Agent Framework\n\n")
		fmt.Fprintf(os.Stderr, "Usage: looper [options]\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nEnvironment Variables:\n")
		fmt.Fprintf(os.Stderr, "  ANTHROPIC_API_KEY      API key for Anthropic Claude\n")
		fmt.Fprintf(os.Stderr, "  OPENAI_API_KEY         API key for OpenAI\n")
		fmt.Fprintf(os.Stderr, "  LOOPER_PROVIDER        Default provider\n")
		fmt.Fprintf(os.Stderr, "  LOOPER_MODEL           Default model\n")
		fmt.Fprintf(os.Stderr, "  LOOPER_WORKSPACE       Default workspace path\n")
		fmt.Fprintf(os.Stderr, "  LOOPER_PROMPTS_PATH    Path to prompts directory\n")
		fmt.Fprintf(os.Stderr, "  LOOPER_SYSTEM_PROMPT   System prompt ID to use\n")
	}

	flag.Parse()

	if *showVersion {
		fmt.Printf("looper version %s\n", version)
		os.Exit(0)
	}

	// Build configuration
	// Priority: CLI flags > env vars > defaults
	config := agent.DefaultConfig()
	config.LoadFromEnv()

	// Override with CLI flags only if explicitly provided
	if *workspace != "" {
		config.WorkspacePath = *workspace
	}
	if *provider != "" {
		config.Provider = *provider
	}
	if *model != "" {
		config.Model = *model
	}
	if *maxIter != 50 {
		config.MaxIterations = *maxIter
	}
	if *systemPrompt != "" {
		config.SystemPrompt = *systemPrompt
	}
	if *systemPromptID != "" {
		config.SystemPromptID = *systemPromptID
	}
	if *promptsPath != "" {
		config.PromptsPath = *promptsPath
	}
	if *disableBlacklist {
		config.DisableBlacklist = true
	}
	if *blacklistFile != "" {
		patterns, err := loadBlacklistFile(*blacklistFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading blacklist file: %v\n", err)
			os.Exit(1)
		}
		config.CommandBlacklist = patterns
	}

	// Create agent
	ag, err := agent.New(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating agent: %v\n", err)
		os.Exit(1)
	}

	// List skills if requested
	if *listSkills {
		skills := ag.Context().LoadedSkills
		if len(skills) == 0 {
			fmt.Println("No skills found in workspace.")
		} else {
			fmt.Println("Loaded Skills:")
			fmt.Println("--------------")
			for name, skill := range skills {
				fmt.Printf("  %s\n    %s\n\n", name, skill.Description)
			}
		}
		os.Exit(0)
	}

	// List prompts if requested
	if *listPrompts {
		promptsList := ag.PromptLoader().GetAll()
		if len(promptsList) == 0 {
			fmt.Println("No prompts found.")
			fmt.Printf("Prompts directory: %s\n", ag.PromptLoader().Directory())
		} else {
			fmt.Println("Loaded Prompts:")
			fmt.Println("---------------")
			for id, p := range promptsList {
				fmt.Printf("  %s%s%s\n", colorCyan, id, colorReset)
				if p.Description != "" {
					fmt.Printf("    %s\n", p.Description)
				}
				if p.SourceFile != "" {
					fmt.Printf("    %sSource: %s%s\n", colorDim, p.SourceFile, colorReset)
				}
				fmt.Println()
			}
		}
		os.Exit(0)
	}

	// Set up context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nInterrupted. Exiting...")
		cancel()
		os.Exit(0)
	}()

	// Run in single prompt mode or interactive mode
	if *prompt != "" {
		runSinglePrompt(ctx, ag, *prompt)
	} else {
		runInteractive(ctx, ag)
	}
}

func runSinglePrompt(ctx context.Context, ag *agent.Agent, prompt string) {
	handler := createStreamHandler()
	_, err := ag.RunStream(ctx, prompt, handler)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n%sError: %v%s\n", colorRed, err, colorReset)
		os.Exit(1)
	}
	fmt.Println()
}

func runInteractive(ctx context.Context, ag *agent.Agent) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Printf("%s%sLooper AI Agent%s\n", colorBold, colorCyan, colorReset)
	fmt.Printf("%s===============%s\n", colorCyan, colorReset)
	fmt.Printf("%sWorkspace:%s %s\n", colorDim, colorReset, ag.Context().WorkspacePath)
	fmt.Printf("%sProvider:%s %s\n", colorDim, colorReset, ag.Registry().Names())
	fmt.Println()
	fmt.Println("Type your message and press Enter. Commands:")
	fmt.Printf("  %s/quit, /exit%s  - Exit the agent\n", colorYellow, colorReset)
	fmt.Printf("  %s/clear%s        - Clear conversation history\n", colorYellow, colorReset)
	fmt.Printf("  %s/skills%s       - List loaded skills\n", colorYellow, colorReset)
	fmt.Printf("  %s/tools%s        - List available tools\n", colorYellow, colorReset)
	fmt.Printf("  %s/prompts%s      - List loaded prompts\n", colorYellow, colorReset)
	fmt.Printf("  %s/help%s         - Show this help\n", colorYellow, colorReset)
	fmt.Println()

	for {
		fmt.Printf("%s%sYou:%s ", colorBold, colorGreen, colorReset)
		input, err := reader.ReadString('\n')
		if err != nil {
			break
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		// Handle commands
		if strings.HasPrefix(input, "/") {
			if handleCommand(ag, input) {
				continue
			}
			return // Exit command
		}

		// Run agent with streaming
		fmt.Println()
		fmt.Printf("%s%sAssistant:%s ", colorBold, colorBlue, colorReset)

		handler := createStreamHandler()
		_, err = ag.RunStream(ctx, input, handler)
		if err != nil {
			if ctx.Err() != nil {
				return // Context cancelled
			}
			fmt.Printf("\n%sError: %v%s\n\n", colorRed, err, colorReset)
			continue
		}

		fmt.Println()

		// Show token usage
		agCtx := ag.Context()
		fmt.Printf("%s[Tokens: %d in / %d out | Iterations: %d]%s\n\n",
			colorDim, agCtx.TotalInputTokens, agCtx.TotalOutputTokens, agCtx.IterationCount, colorReset)
	}
}

// createStreamHandler creates a StreamHandler with colored output
func createStreamHandler() *agent.StreamHandler {
	return &agent.StreamHandler{
		OnText: func(text string) {
			fmt.Print(text)
		},
		OnToolStart: func(tc llm.ToolCall) {
			fmt.Printf("\n\n%s%s▶ Tool Call: %s%s\n", colorBold, colorMagenta, tc.Name, colorReset)
			// Pretty print the arguments
			var args map[string]interface{}
			if err := json.Unmarshal(tc.Arguments, &args); err == nil {
				prettyArgs, _ := json.MarshalIndent(args, "  ", "  ")
				fmt.Printf("  %s%s%s\n", colorDim, string(prettyArgs), colorReset)
			} else {
				fmt.Printf("  %s%s%s\n", colorDim, string(tc.Arguments), colorReset)
			}
		},
		OnToolEnd: func(tc llm.ToolCall, result string, err error) {
			if err != nil {
				fmt.Printf("%s%s✗ Error: %s%s\n", colorBold, colorRed, err.Error(), colorReset)
			} else {
				// Truncate long results for display
				displayResult := result
				if len(displayResult) > 500 {
					displayResult = displayResult[:500] + "... (truncated)"
				}
				// Replace newlines with indented newlines for readability
				displayResult = strings.ReplaceAll(displayResult, "\n", "\n  ")
				fmt.Printf("%s%s✓ Result:%s\n  %s%s%s\n", colorBold, colorGreen, colorReset, colorDim, displayResult, colorReset)
			}
			fmt.Printf("\n%s%sAssistant:%s ", colorBold, colorBlue, colorReset)
		},
		OnUsage: func(inputTokens, outputTokens int) {
			// Usage is displayed after the loop
		},
		OnDone: func() {
			// Done
		},
	}
}

// handleCommand processes CLI commands. Returns false if should exit.
func handleCommand(ag *agent.Agent, input string) bool {
	parts := strings.Fields(input)
	cmd := strings.ToLower(parts[0])

	switch cmd {
	case "/quit", "/exit":
		fmt.Println("Goodbye!")
		return false

	case "/clear":
		ag.Reset()
		fmt.Println("Conversation cleared.")
		fmt.Println()
		return true

	case "/skills":
		skills := ag.Context().LoadedSkills
		if len(skills) == 0 {
			fmt.Println("No skills loaded.")
			fmt.Println()
		} else {
			fmt.Println("Loaded Skills:")
			for name, skill := range skills {
				fmt.Printf("  - %s: %s\n", name, skill.Description)
			}
			fmt.Println()
		}
		return true

	case "/tools":
		tools := ag.Registry().Names()
		fmt.Println("Available Tools:")
		for _, name := range tools {
			fmt.Printf("  - %s\n", name)
		}
		fmt.Println()
		return true

	case "/prompts":
		promptsList := ag.PromptLoader().GetAll()
		if len(promptsList) == 0 {
			fmt.Println("No prompts loaded.")
			fmt.Println()
		} else {
			fmt.Println("Loaded Prompts:")
			for id, p := range promptsList {
				fmt.Printf("  - %s%s%s", colorCyan, id, colorReset)
				if p.Description != "" {
					fmt.Printf(": %s", p.Description)
				}
				fmt.Println()
			}
			fmt.Println()
		}
		return true

	case "/help":
		fmt.Println("Commands:")
		fmt.Println("  /quit, /exit  - Exit the agent")
		fmt.Println("  /clear        - Clear conversation history")
		fmt.Println("  /skills       - List loaded skills")
		fmt.Println("  /tools        - List available tools")
		fmt.Println("  /prompts      - List loaded prompts")
		fmt.Println("  /help         - Show this help")
		fmt.Println()
		return true

	default:
		fmt.Printf("Unknown command: %s\n\n", cmd)
		return true
	}
}

// loadBlacklistFile reads a blacklist file with one pattern per line
func loadBlacklistFile(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var patterns []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return patterns, nil
}
