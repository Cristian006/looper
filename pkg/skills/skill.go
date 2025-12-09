package skills

// Skill represents a loaded skill with its metadata and content
type Skill struct {
	// Name is the unique identifier for the skill
	Name string `yaml:"name" json:"name"`

	// Description describes what the skill does and when to use it
	Description string `yaml:"description" json:"description"`

	// Content is the markdown content of the skill (instructions, examples, etc.)
	Content string `json:"content"`

	// FilePath is the path to the skill file
	FilePath string `json:"file_path"`
}

// Frontmatter represents the YAML frontmatter of a skill file
type Frontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// ToPrompt converts the skill to a reference string (name, description, path only)
func (s *Skill) ToPrompt() string {
	return "- **" + s.Name + "** (`" + s.FilePath + "`): " + s.Description
}
