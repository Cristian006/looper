package sandbox

import (
	"context"
	"time"
)

// ExecutionResult contains the result of a sandboxed execution
type ExecutionResult struct {
	Stdout   string        `json:"stdout"`
	Stderr   string        `json:"stderr"`
	ExitCode int           `json:"exit_code"`
	Duration time.Duration `json:"duration"`
	TimedOut bool          `json:"timed_out"`
}

// Sandbox is the interface for sandboxed code execution
type Sandbox interface {
	// Execute runs a command in the sandbox
	Execute(ctx context.Context, command string, args []string) (*ExecutionResult, error)

	// ExecuteScript runs a script in the sandbox
	ExecuteScript(ctx context.Context, interpreter string, script string) (*ExecutionResult, error)

	// WorkingDir returns the sandbox working directory
	WorkingDir() string
}

// Config holds sandbox configuration
type Config struct {
	WorkingDir       string            // Working directory for execution
	Timeout          time.Duration     // Maximum execution time
	AllowedEnv       []string          // Environment variables to pass through
	CustomEnv        map[string]string // Custom environment variables to set
	MaxOutputBytes   int64             // Maximum output size in bytes
	CommandBlacklist []string          // Patterns to block (supports wildcards)
}

// DefaultConfig returns a default sandbox configuration
func DefaultConfig(workingDir string) *Config {
	return &Config{
		WorkingDir:     workingDir,
		Timeout:        30 * time.Second,
		MaxOutputBytes: 1024 * 1024, // 1MB
		AllowedEnv: []string{
			"PATH",
			"HOME",
			"USER",
			"LANG",
			"LC_ALL",
		},
		CustomEnv:        make(map[string]string),
		CommandBlacklist: DefaultBlacklist(),
	}
}

// DefaultBlacklist returns a default list of dangerous command patterns
func DefaultBlacklist() []string {
	return []string{
		// Destructive file operations
		"rm -rf /",
		"rm -rf /*",
		"rm -rf ~",
		"rm -rf .",
		"rm -rf ..",
		"rm -fr /",
		"rm -fr /*",
		"> /dev/sda",
		"dd if=/dev/zero of=/dev/sda",
		"dd if=/dev/random of=/dev/sda",
		"mkfs.",
		"wipefs",

		// Fork bombs and resource exhaustion
		":(){ :|:& };:",
		"fork while fork",

		// System manipulation
		"chmod -R 777 /",
		"chown -R",
		"shutdown",
		"reboot",
		"halt",
		"poweroff",
		"init 0",
		"init 6",
		"telinit 0",

		// Network attacks
		"nc -l", // Netcat listener (could be used for reverse shells)

		// Dangerous downloads and execution
		"curl * | sh",
		"curl * | bash",
		"wget * | sh",
		"wget * | bash",

		// History/log tampering
		"history -c",
		"cat /dev/null >",
		"> ~/.bash_history",

		// Privilege escalation attempts
		"sudo su",
		"sudo -i",
		"su -",
		"passwd",

		// Crypto mining indicators
		"xmrig",
		"minerd",
		"cpuminer",

		// Kernel manipulation
		"insmod",
		"rmmod",
		"modprobe",
	}
}
