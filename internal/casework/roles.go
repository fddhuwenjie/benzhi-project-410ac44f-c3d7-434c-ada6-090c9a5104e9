package casework

import "strings"

type Role string

const (
	RoleDuty         Role = "duty"
	RoleEngineer     Role = "engineer"
	RoleInvestigator Role = "investigator"
	RoleReviewer     Role = "reviewer"
)

func ParseActorRole(actor string) (Role, string) {
	parts := strings.SplitN(strings.TrimSpace(actor), ":", 2)
	if len(parts) == 2 {
		return Role(parts[0]), parts[1]
	}
	return "", actor
}

func CanClose(actor string) bool {
	role, name := ParseActorRole(actor)
	return name != "" && role == RoleReviewer
}
