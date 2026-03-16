package review

// Role identifies a reviewer's specialization.
type Role string

const (
	RoleBugHunter    Role = "bug_hunter"
	RoleArchSecurity Role = "arch_security"
)

// RoleForCLI returns the default role assigned to each CLI in mega mode.
func RoleForCLI(cli string) Role {
	switch cli {
	case "codex":
		return RoleBugHunter
	case "gemini":
		return RoleArchSecurity
	default:
		return RoleBugHunter
	}
}
