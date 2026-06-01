package skills

// Skill describes one local SKILL.md instruction file.
type Skill struct {
	Name        string
	Description string
	Path        string
}

// LoadError records a malformed or unreadable skill file.
type LoadError struct {
	Path    string
	Message string
}

// Catalog is the skills snapshot available to one agent turn.
type Catalog struct {
	Skills []Skill
	Errors []LoadError
}

// Injection is the full skill body selected for one turn.
type Injection struct {
	Name     string
	Path     string
	Contents string
}

// PromptContext contains rendered prompt fragments derived from a catalog.
type PromptContext struct {
	Available string
	Injected  []Injection
	Warnings  []string
}
