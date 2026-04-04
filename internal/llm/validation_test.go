package llm

import "testing"

func TestValidatePartRequiresOnePayload(t *testing.T) {
	part := Part{Type: PartText}
	if err := ValidatePart(part); err == nil {
		t.Fatal("expected one-of validation error")
	}
}

func TestValidateMessageRejectsInvalidRolePartCombination(t *testing.T) {
	msg := Message{
		Role: RoleSystem,
		Parts: []Part{{
			Type:    PartToolUse,
			ToolUse: &ToolUsePart{ID: "call-1", Name: "x", Arguments: "{}"},
		}},
	}
	if err := ValidateMessage(msg); err == nil {
		t.Fatal("expected role-part validation error")
	}
}

func TestValidateMessageAllowsUserToolResult(t *testing.T) {
	msg := NewToolResultMessage("call-1", `{"ok":true}`)
	if err := ValidateMessage(msg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
