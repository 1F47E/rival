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
	default:
		return RoleBugHunter
	}
}
