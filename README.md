# Looper - Go AI Agent Framework

A modular AI agent framework written in Go with pluggable LLM providers, sandboxed tool execution, and skill discovery.

## Features

- **Pluggable LLM Providers**: Support for Anthropic Claude and OpenAI APIs
- **Built-in Tools**: File operations (read, write, grep) and sandboxed code execution
- **Skills System**: Markdown-based skill files with YAML frontmatter for progressive discovery
- **Process Sandbox**: Isolated code execution with timeout and permission controls

## Installation

```bash
go install github.com/looper-ai/looper/cmd/looper@latest
```

Or build from source:

```bash
git clone https://github.com/looper-ai/looper.git
cd looper
go build -o looper ./cmd/looper
```

## Usage

### Interactive Mode

```bash
looper --workspace ./my-project
```

### Single Prompt

```bash
looper --prompt "Read the main.go file and explain what it does" --workspace ./my-project
```

### Configuration

Set your API keys via environment variables:

```bash
export ANTHROPIC_API_KEY="your-key-here"
export OPENAI_API_KEY="your-key-here"
```

Select provider and model:

```bash
looper --provider anthropic --model claude-sonnet-4-20250514
looper --provider openai --model gpt-4o
```

## Skills

Skills are markdown files with YAML frontmatter that extend the agent's capabilities:

```markdown
---
name: my-skill
description: A clear description of what this skill does and when to use it
---
# My Skill Name

Instructions that the agent will follow when this skill is active.

## Examples
- Example usage 1
- Example usage 2

## Guidelines
- Guideline 1
- Guideline 2
```

Place skill files in a `skills/` directory within your workspace. The agent will progressively discover and load them as needed.

## Project Structure

```
looper/
├── cmd/looper/main.go           # CLI entry point
├── pkg/
│   ├── agent/                   # Core agent loop and state
│   ├── llm/                     # Pluggable LLM providers
│   ├── tools/                   # Agent tools (grep, file ops, execute)
│   ├── sandbox/                 # Process-based sandboxing
│   └── skills/                  # Skill loading and discovery
├── go.mod
└── README.md
```

## License

MIT License

