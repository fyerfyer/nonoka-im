package domain

import "time"

// MCPServer is a platform-registered trusted MCP server.
type MCPServer struct {
	ID           uint64
	Name         string
	Transport    string
	URL          string
	AllowedTools string
	RiskClass    string
	Enabled      bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Skill is a platform-published, reviewed SKILL.md version.
type Skill struct {
	ID        uint64
	Name      string
	Version   int64
	Content   string
	Published bool
	CreatedAt time.Time
	UpdatedAt time.Time
}
