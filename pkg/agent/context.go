package agent

import (
	"github.com/looper-ai/looper/pkg/llm"
	"github.com/looper-ai/looper/pkg/skills"
)

// Context holds the state of an agent conversation
type Context struct {
	// Messages is the conversation history
	Messages []llm.Message

	// LoadedSkills contains skills that have been activated
	LoadedSkills map[string]*skills.Skill

	// WorkspacePath is the root directory for operations
	WorkspacePath string

	// Metadata holds arbitrary context data
	Metadata map[string]interface{}

	// TotalInputTokens tracks cumulative input tokens
	TotalInputTokens int

	// TotalOutputTokens tracks cumulative output tokens
	TotalOutputTokens int

	// IterationCount tracks the number of tool call iterations
	IterationCount int
}

// NewContext creates a new agent context
func NewContext(workspacePath string) *Context {
	return &Context{
		Messages:      make([]llm.Message, 0),
		LoadedSkills:  make(map[string]*skills.Skill),
		WorkspacePath: workspacePath,
		Metadata:      make(map[string]interface{}),
	}
}

// AddMessage appends a message to the conversation
func (c *Context) AddMessage(msg llm.Message) {
	c.Messages = append(c.Messages, msg)
}

// AddUserMessage adds a user message
func (c *Context) AddUserMessage(content string) {
	c.AddMessage(llm.NewUserMessage(content))
}

// AddAssistantMessage adds an assistant message
func (c *Context) AddAssistantMessage(content string) {
	c.AddMessage(llm.NewAssistantMessage(content))
}

// AddToolResult adds a tool result message
func (c *Context) AddToolResult(toolCallID, content string) {
	c.AddMessage(llm.NewToolResultMessage(toolCallID, content))
}

// LoadSkill adds a skill to the context
func (c *Context) LoadSkill(skill *skills.Skill) {
	if skill != nil {
		c.LoadedSkills[skill.Name] = skill
	}
}

// GetSkillPrompt returns the skill references for the system prompt
// Only includes name, description, and file path - agent can read_file for full content
func (c *Context) GetSkillPrompt() string {
	if len(c.LoadedSkills) == 0 {
		return ""
	}

	prompt := "\n\n## Available Skills\n"
	prompt += "Use `read_file` to view full skill instructions when needed:\n\n"
	for _, skill := range c.LoadedSkills {
		prompt += skill.ToPrompt() + "\n"
	}
	return prompt
}

// UpdateUsage updates token usage statistics
func (c *Context) UpdateUsage(usage llm.Usage) {
	c.TotalInputTokens += usage.InputTokens
	c.TotalOutputTokens += usage.OutputTokens
}

// GetLastAssistantMessage returns the last assistant message, if any
func (c *Context) GetLastAssistantMessage() *llm.Message {
	for i := len(c.Messages) - 1; i >= 0; i-- {
		if c.Messages[i].Role == llm.RoleAssistant {
			return &c.Messages[i]
		}
	}
	return nil
}

// Clear resets the conversation while preserving workspace and skills
func (c *Context) Clear() {
	c.Messages = make([]llm.Message, 0)
	c.IterationCount = 0
}

// Clone creates a copy of the context
func (c *Context) Clone() *Context {
	clone := &Context{
		Messages:          make([]llm.Message, len(c.Messages)),
		LoadedSkills:      make(map[string]*skills.Skill),
		WorkspacePath:     c.WorkspacePath,
		Metadata:          make(map[string]interface{}),
		TotalInputTokens:  c.TotalInputTokens,
		TotalOutputTokens: c.TotalOutputTokens,
		IterationCount:    c.IterationCount,
	}

	copy(clone.Messages, c.Messages)

	for k, v := range c.LoadedSkills {
		clone.LoadedSkills[k] = v
	}

	for k, v := range c.Metadata {
		clone.Metadata[k] = v
	}

	return clone
}
