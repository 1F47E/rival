package review

// Role identifies a reviewer's specialization.
type Role string

const (
	RoleBugHunter    Role = "bug_hunter"
	RoleArchSecurity Role = "arch_security"
	RoleCodeQuality  Role = "code_quality"
)

// RoleForCLI returns the fallback role for a CLI-backed review session.
func RoleForCLI(cli string) Role {
	switch cli {
	case "codex":
		return RoleBugHunter
	case "gemini":
		return RoleArchSecurity
	case "claude":
		return RoleCodeQuality
	case "antigravity":
		return RoleBugHunter
	case "opencode":
		// The generic OpenCode fallback model is DeepSeek V4 Pro.
		return RoleBugHunter
	default:
		return RoleBugHunter
	}
}
