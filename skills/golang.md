---
name: golang
description: Best practices and conventions for writing Go code
---
# Go Development Skill

Follow these guidelines when working with Go code.

## Code Style

- Use `gofmt` formatting conventions
- Name packages with short, lowercase names without underscores
- Use MixedCaps or mixedCaps for multi-word names
- Keep functions focused and small
- Prefer composition over inheritance

## Error Handling

- Always check errors - don't ignore them
- Return errors rather than panicking
- Wrap errors with context using `fmt.Errorf("context: %w", err)`
- Use custom error types when callers need to handle specific cases

## Examples

- "Write a function to parse JSON config files"
- "Create a concurrent worker pool"
- "Implement an HTTP handler with proper error handling"

## Guidelines

- Use `context.Context` for cancellation and timeouts
- Prefer interfaces for dependencies (dependency injection)
- Write table-driven tests
- Use `defer` for cleanup operations
- Document exported functions and types

## Common Patterns

```go
// Error wrapping
if err != nil {
    return fmt.Errorf("failed to process: %w", err)
}

// Context with timeout
ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
defer cancel()

// Defer for cleanup
file, err := os.Open(path)
if err != nil {
    return err
}
defer file.Close()
```

