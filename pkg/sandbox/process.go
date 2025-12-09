package sandbox

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ErrBlacklistedCommand is returned when a command matches a blacklist pattern
var ErrBlacklistedCommand = errors.New("command blocked by blacklist")

// ProcessSandbox implements Sandbox using process-level isolation
type ProcessSandbox struct {
	config *Config
}

// NewProcessSandbox creates a new process-based sandbox
func NewProcessSandbox(config *Config) *ProcessSandbox {
	if config == nil {
		config = DefaultConfig(".")
	}
	return &ProcessSandbox{
		config: config,
	}
}

func (s *ProcessSandbox) WorkingDir() string {
	return s.config.WorkingDir
}

// checkBlacklist checks if the command or script contains blacklisted patterns
func (s *ProcessSandbox) checkBlacklist(input string) error {
	if len(s.config.CommandBlacklist) == 0 {
		return nil
	}

	// Normalize input for checking
	normalizedInput := strings.ToLower(input)
	// Remove extra whitespace
	normalizedInput = regexp.MustCompile(`\s+`).ReplaceAllString(normalizedInput, " ")

	for _, pattern := range s.config.CommandBlacklist {
		normalizedPattern := strings.ToLower(pattern)

		// Convert glob-style wildcards to regex
		// Escape regex special chars except *
		escaped := regexp.QuoteMeta(normalizedPattern)
		// Convert * back to regex .*
		regexPattern := strings.ReplaceAll(escaped, `\*`, `.*`)

		re, err := regexp.Compile(regexPattern)
		if err != nil {
			// If pattern is invalid, do simple substring match
			if strings.Contains(normalizedInput, normalizedPattern) {
				return fmt.Errorf("%w: matches pattern %q", ErrBlacklistedCommand, pattern)
			}
			continue
		}

		if re.MatchString(normalizedInput) {
			return fmt.Errorf("%w: matches pattern %q", ErrBlacklistedCommand, pattern)
		}
	}

	return nil
}

func (s *ProcessSandbox) Execute(ctx context.Context, command string, args []string) (*ExecutionResult, error) {
	// Build full command string for blacklist checking
	fullCommand := command + " " + strings.Join(args, " ")
	if err := s.checkBlacklist(fullCommand); err != nil {
		return nil, err
	}

	// Apply timeout
	if s.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.config.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, command, args...)
	return s.runCommand(ctx, cmd)
}

func (s *ProcessSandbox) ExecuteScript(ctx context.Context, interpreter string, script string) (*ExecutionResult, error) {
	// Check script content against blacklist
	if err := s.checkBlacklist(script); err != nil {
		return nil, err
	}

	// Apply timeout
	if s.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.config.Timeout)
		defer cancel()
	}

	// For Python, wrap the script to behave like a REPL (auto-print expressions)
	if interpreter == "python" || interpreter == "python3" {
		script = wrapPythonScript(script)
	}

	// Create temporary script file
	tmpDir := os.TempDir()
	var ext string
	switch interpreter {
	case "python", "python3":
		ext = ".py"
	case "node", "nodejs":
		ext = ".js"
	case "bash", "sh":
		ext = ".sh"
	case "go":
		ext = ".go"
	default:
		ext = ".tmp"
	}

	tmpFile, err := os.CreateTemp(tmpDir, "looper-script-*"+ext)
	if err != nil {
		return nil, fmt.Errorf("failed to create temp script: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.WriteString(script); err != nil {
		tmpFile.Close()
		return nil, fmt.Errorf("failed to write script: %w", err)
	}
	tmpFile.Close()

	// Make script executable for shell scripts
	if interpreter == "bash" || interpreter == "sh" {
		os.Chmod(tmpPath, 0755)
	}

	// Build command based on interpreter
	var cmd *exec.Cmd
	switch interpreter {
	case "go":
		cmd = exec.CommandContext(ctx, "go", "run", tmpPath)
	default:
		cmd = exec.CommandContext(ctx, interpreter, tmpPath)
	}

	return s.runCommand(ctx, cmd)
}

func (s *ProcessSandbox) runCommand(ctx context.Context, cmd *exec.Cmd) (*ExecutionResult, error) {
	// Set working directory
	absWorkDir, err := filepath.Abs(s.config.WorkingDir)
	if err != nil {
		return nil, fmt.Errorf("invalid working directory: %w", err)
	}
	cmd.Dir = absWorkDir

	// Set up environment
	env := s.buildEnvironment()
	cmd.Env = env

	// Set up output capture with size limits
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &limitedWriter{w: &stdout, limit: s.config.MaxOutputBytes}
	cmd.Stderr = &limitedWriter{w: &stderr, limit: s.config.MaxOutputBytes}

	// Run command
	startTime := time.Now()
	err = cmd.Run()
	duration := time.Since(startTime)

	result := &ExecutionResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: duration,
	}

	// Check for timeout
	if ctx.Err() == context.DeadlineExceeded {
		result.TimedOut = true
		result.ExitCode = -1
		return result, nil
	}

	// Get exit code
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("execution failed: %w", err)
		}
	}

	return result, nil
}

func (s *ProcessSandbox) buildEnvironment() []string {
	env := make([]string, 0)

	// Copy allowed environment variables
	for _, key := range s.config.AllowedEnv {
		if val, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+val)
		}
	}

	// Add custom environment variables
	for key, val := range s.config.CustomEnv {
		env = append(env, key+"="+val)
	}

	// Ensure PATH includes common binary locations
	hasPath := false
	for _, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			hasPath = true
			break
		}
	}
	if !hasPath {
		env = append(env, "PATH=/usr/local/bin:/usr/bin:/bin")
	}

	return env
}

// limitedWriter wraps a writer and limits the amount of data written
type limitedWriter struct {
	w       io.Writer
	limit   int64
	written int64
}

func (lw *limitedWriter) Write(p []byte) (n int, err error) {
	if lw.written >= lw.limit {
		return len(p), nil // Silently discard
	}

	remaining := lw.limit - lw.written
	if int64(len(p)) > remaining {
		p = p[:remaining]
	}

	n, err = lw.w.Write(p)
	lw.written += int64(n)
	return len(p), err // Report full length written to avoid breaking callers
}

// wrapPythonScript wraps Python code to auto-print expression results like a REPL
func wrapPythonScript(script string) string {
	// Check if the script is simple enough to benefit from REPL-style output
	// Multi-line scripts or scripts with explicit prints don't need wrapping
	trimmed := strings.TrimSpace(script)

	// If the script contains print statements or is multi-line with statements,
	// don't wrap it - execute as-is
	if strings.Contains(trimmed, "print(") ||
		strings.Contains(trimmed, "print ") ||
		strings.Contains(trimmed, "import ") ||
		strings.Contains(trimmed, "def ") ||
		strings.Contains(trimmed, "class ") ||
		strings.Contains(trimmed, "if ") ||
		strings.Contains(trimmed, "for ") ||
		strings.Contains(trimmed, "while ") ||
		strings.Contains(trimmed, "with ") ||
		strings.Contains(trimmed, "try:") {
		return script
	}

	// For simple expressions, wrap to auto-print the result
	// Use base64 to safely embed the code
	encoded := base64.StdEncoding.EncodeToString([]byte(script))

	wrapper := fmt.Sprintf(`import base64
_code = base64.b64decode("%s").decode("utf-8")
try:
    _result = eval(compile(_code, '<input>', 'eval'))
    if _result is not None:
        print(repr(_result))
except SyntaxError:
    exec(compile(_code, '<input>', 'exec'))
`, encoded)

	return wrapper
}
