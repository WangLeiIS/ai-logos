package skill

type Skill struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Path        string  `json:"path"`
	Weight      float64 `json:"weight"`
	ArchivedAt  *string `json:"archived_at,omitempty"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

type ValidatedSkill struct {
	Dir          string
	ResourcePath string // "Resources/skills/<name>"
	Name         string
	Description  string
}
