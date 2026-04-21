package extensions

import "strings"

// SkillExtensionID builds the canonical extension id from a skill name.
// It intentionally prefixes with "skill." even when the input already has
// that prefix to keep backward compatibility with existing extension ids.
func SkillExtensionID(skillName string) string {
	name := strings.TrimSpace(skillName)
	if name == "" {
		return ""
	}
	return "skill." + name
}
