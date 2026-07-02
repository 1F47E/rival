package review

// Role identifies a reviewer's specialization.
type Role string

const (
	RoleBugHunter    Role = "bug_hunter"
	RoleArchSecurity Role = "arch_security"
	RoleCodeQuality  Role = "code_quality"
)

// RoleForCLI returns the default role assigned to each CLI in mega mode.
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
		// codex + antigravity both hunt bugs; give opencode (GLM) the
		// architecture/security lens to diversify the 3-reviewer roster.
		return RoleArchSecurity
	default:
		return RoleBugHunter
	}
}
