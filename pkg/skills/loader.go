package skills

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Loader handles loading skill files
type Loader struct{}

// NewLoader creates a new skill loader
func NewLoader() *Loader {
	return &Loader{}
}

// Load reads and parses a skill file
func (l *Loader) Load(filePath string) (*Skill, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open skill file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	// Check for frontmatter start
	if !scanner.Scan() {
		return nil, fmt.Errorf("empty skill file")
	}

	firstLine := scanner.Text()
	if strings.TrimSpace(firstLine) != "---" {
		return nil, fmt.Errorf("skill file must start with YAML frontmatter (---)")
	}

	// Read frontmatter
	var frontmatterLines []string
	foundEnd := false

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			foundEnd = true
			break
		}
		frontmatterLines = append(frontmatterLines, line)
	}

	if !foundEnd {
		return nil, fmt.Errorf("unclosed frontmatter (missing closing ---)")
	}

	// Parse frontmatter
	frontmatterYAML := strings.Join(frontmatterLines, "\n")
	var frontmatter Frontmatter
	if err := yaml.Unmarshal([]byte(frontmatterYAML), &frontmatter); err != nil {
		return nil, fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	// Validate required fields
	if frontmatter.Name == "" {
		return nil, fmt.Errorf("skill frontmatter must have a 'name' field")
	}
	if frontmatter.Description == "" {
		return nil, fmt.Errorf("skill frontmatter must have a 'description' field")
	}

	// Read content (everything after frontmatter)
	var contentLines []string
	for scanner.Scan() {
		contentLines = append(contentLines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading skill file: %w", err)
	}

	// Trim leading empty lines from content
	content := strings.TrimLeft(strings.Join(contentLines, "\n"), "\n")

	return &Skill{
		Name:        frontmatter.Name,
		Description: frontmatter.Description,
		Content:     content,
		FilePath:    filePath,
	}, nil
}

// LoadFromString parses a skill from a string (useful for testing)
func (l *Loader) LoadFromString(content string, filePath string) (*Skill, error) {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return nil, fmt.Errorf("empty skill content")
	}

	// Check for frontmatter start
	if strings.TrimSpace(lines[0]) != "---" {
		return nil, fmt.Errorf("skill must start with YAML frontmatter (---)")
	}

	// Find frontmatter end
	frontmatterEnd := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			frontmatterEnd = i
			break
		}
	}

	if frontmatterEnd == -1 {
		return nil, fmt.Errorf("unclosed frontmatter (missing closing ---)")
	}

	// Parse frontmatter
	frontmatterYAML := strings.Join(lines[1:frontmatterEnd], "\n")
	var frontmatter Frontmatter
	if err := yaml.Unmarshal([]byte(frontmatterYAML), &frontmatter); err != nil {
		return nil, fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	// Validate required fields
	if frontmatter.Name == "" {
		return nil, fmt.Errorf("skill frontmatter must have a 'name' field")
	}
	if frontmatter.Description == "" {
		return nil, fmt.Errorf("skill frontmatter must have a 'description' field")
	}

	// Get content
	bodyContent := ""
	if frontmatterEnd+1 < len(lines) {
		bodyContent = strings.TrimLeft(strings.Join(lines[frontmatterEnd+1:], "\n"), "\n")
	}

	return &Skill{
		Name:        frontmatter.Name,
		Description: frontmatter.Description,
		Content:     bodyContent,
		FilePath:    filePath,
	}, nil
}
