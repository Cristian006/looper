package skills

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Discovery handles finding and loading skills from a workspace
type Discovery struct {
	workspaceRoot string
	skillsDir     string
	loader        *Loader
	mu            sync.RWMutex
	skills        map[string]*Skill // Loaded skills by name
	fileIndex     map[string]string // Map of skill name to file path
	discovered    bool              // Whether discovery has been performed
}

// NewDiscovery creates a new skill discovery instance
func NewDiscovery(workspaceRoot string) *Discovery {
	return &Discovery{
		workspaceRoot: workspaceRoot,
		skillsDir:     filepath.Join(workspaceRoot, "skills"),
		loader:        NewLoader(),
		skills:        make(map[string]*Skill),
		fileIndex:     make(map[string]string),
	}
}

// SetSkillsDir sets a custom skills directory
func (d *Discovery) SetSkillsDir(dir string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.skillsDir = dir
	d.discovered = false
	d.skills = make(map[string]*Skill)
	d.fileIndex = make(map[string]string)
}

// Discover scans the skills directory and indexes available skills
// This performs lazy discovery - it finds skill files but doesn't load them
func (d *Discovery) Discover() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Check if skills directory exists
	if _, err := os.Stat(d.skillsDir); os.IsNotExist(err) {
		d.discovered = true
		return nil // No skills directory is fine
	}

	// Walk the skills directory
	err := filepath.Walk(d.skillsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files we can't access
		}

		// Skip directories
		if info.IsDir() {
			// Skip hidden directories
			if strings.HasPrefix(info.Name(), ".") && path != d.skillsDir {
				return filepath.SkipDir
			}
			return nil
		}

		// Only process .md files
		if !strings.HasSuffix(info.Name(), ".md") {
			return nil
		}

		// Try to extract skill name from frontmatter without fully loading
		skillName := d.extractSkillName(path)
		if skillName != "" {
			d.fileIndex[skillName] = path
		}

		return nil
	})

	d.discovered = true
	return err
}

// extractSkillName reads just enough of the file to get the skill name
func (d *Discovery) extractSkillName(filePath string) string {
	file, err := os.Open(filePath)
	if err != nil {
		return ""
	}
	defer file.Close()

	// Read first 1KB to find frontmatter
	buf := make([]byte, 1024)
	n, err := file.Read(buf)
	if err != nil || n == 0 {
		return ""
	}

	content := string(buf[:n])
	lines := strings.Split(content, "\n")

	// Check for frontmatter start
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return ""
	}

	// Find name field in frontmatter
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "---" {
			break
		}
		if strings.HasPrefix(line, "name:") {
			name := strings.TrimSpace(strings.TrimPrefix(line, "name:"))
			// Remove quotes if present
			name = strings.Trim(name, "\"'")
			return name
		}
	}

	return ""
}

// List returns a list of available skill names (without loading them)
func (d *Discovery) List() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if !d.discovered {
		d.mu.RUnlock()
		d.Discover()
		d.mu.RLock()
	}

	names := make([]string, 0, len(d.fileIndex))
	for name := range d.fileIndex {
		names = append(names, name)
	}
	return names
}

// SkillInfo contains basic info about a skill for display
type SkillInfo struct {
	Name        string
	Description string
	FilePath    string
}

// ListWithDescriptions returns skills with their descriptions
func (d *Discovery) ListWithDescriptions() map[string]string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if !d.discovered {
		d.mu.RUnlock()
		d.Discover()
		d.mu.RLock()
	}

	result := make(map[string]string)
	for name, path := range d.fileIndex {
		// Try to get from cache first
		if skill, ok := d.skills[name]; ok {
			result[name] = skill.Description
			continue
		}

		// Load skill to get description
		d.mu.RUnlock()
		skill, err := d.Get(name)
		d.mu.RLock()

		if err == nil && skill != nil {
			result[name] = skill.Description
		} else {
			result[name] = "(error loading skill from " + path + ")"
		}
	}

	return result
}

// ListWithInfo returns skills with their descriptions and file paths
func (d *Discovery) ListWithInfo() []SkillInfo {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if !d.discovered {
		d.mu.RUnlock()
		d.Discover()
		d.mu.RLock()
	}

	result := make([]SkillInfo, 0, len(d.fileIndex))
	for name, path := range d.fileIndex {
		info := SkillInfo{
			Name:     name,
			FilePath: d.getRelativePath(path),
		}

		// Try to get description from cache first
		if skill, ok := d.skills[name]; ok {
			info.Description = skill.Description
		} else {
			// Load skill to get description
			d.mu.RUnlock()
			skill, err := d.Get(name)
			d.mu.RLock()

			if err == nil && skill != nil {
				info.Description = skill.Description
			} else {
				info.Description = "(error loading)"
			}
		}

		result = append(result, info)
	}

	return result
}

// getRelativePath returns the path relative to workspace root
func (d *Discovery) getRelativePath(fullPath string) string {
	rel, err := filepath.Rel(d.workspaceRoot, fullPath)
	if err != nil {
		return fullPath
	}
	return rel
}

// Get retrieves a skill by name, loading it if necessary
func (d *Discovery) Get(name string) (*Skill, error) {
	d.mu.RLock()
	if skill, ok := d.skills[name]; ok {
		d.mu.RUnlock()
		return skill, nil
	}
	d.mu.RUnlock()

	// Check if we need to discover
	d.mu.RLock()
	if !d.discovered {
		d.mu.RUnlock()
		if err := d.Discover(); err != nil {
			return nil, err
		}
		d.mu.RLock()
	}

	// Find file path
	filePath, ok := d.fileIndex[name]
	d.mu.RUnlock()

	if !ok {
		return nil, nil // Skill not found
	}

	// Load the skill
	skill, err := d.loader.Load(filePath)
	if err != nil {
		return nil, err
	}

	// Cache it
	d.mu.Lock()
	d.skills[name] = skill
	d.mu.Unlock()

	return skill, nil
}

// GetAll loads and returns all discovered skills
func (d *Discovery) GetAll() ([]*Skill, error) {
	names := d.List()
	skills := make([]*Skill, 0, len(names))

	for _, name := range names {
		skill, err := d.Get(name)
		if err != nil {
			continue // Skip skills that fail to load
		}
		if skill != nil {
			skills = append(skills, skill)
		}
	}

	return skills, nil
}

// Refresh clears the cache and re-discovers skills
func (d *Discovery) Refresh() error {
	d.mu.Lock()
	d.skills = make(map[string]*Skill)
	d.fileIndex = make(map[string]string)
	d.discovered = false
	d.mu.Unlock()

	return d.Discover()
}

// SkillsDir returns the skills directory path
func (d *Discovery) SkillsDir() string {
	return d.skillsDir
}
