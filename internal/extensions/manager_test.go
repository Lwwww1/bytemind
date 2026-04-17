package extensions

import (
	"errors"
	"testing"
)

func TestNopManagerLoad(t *testing.T) {
	mgr := NopManager{}
	item, err := mgr.Load(nil, ".bytemind/skills/review")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if !item.IsZero() {
		t.Fatal("expected zero extension info")
	}
}

func TestNopManagerUnload(t *testing.T) {
	mgr := NopManager{}
	if err := mgr.Unload(nil, "skill.review"); err != nil {
		t.Fatalf("Unload failed: %v", err)
	}
}

func TestNopManagerGet(t *testing.T) {
	mgr := NopManager{}
	item, err := mgr.Get(nil, "skill.review")
	if item != (ExtensionInfo{}) {
		t.Fatal("expected zero extension info")
	}
	if err == nil {
		t.Fatal("expected not found error")
	}
	var extErr *ExtensionError
	if !errors.As(err, &extErr) {
		t.Fatalf("expected ExtensionError, got %T", err)
	}
	if extErr.Code != ErrCodeNotFound {
		t.Fatalf("unexpected code: %s", extErr.Code)
	}
}

func TestNopManagerList(t *testing.T) {
	mgr := NopManager{}
	items, err := mgr.List(nil)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected no extensions, got %d", len(items))
	}
}

func TestExtensionInfoValid(t *testing.T) {
	valid := ExtensionInfo{
		ID:   "skill.review",
		Name: "review",
		Kind: ExtensionSkill,
		Source: ExtensionSource{
			Scope: ExtensionScopeProject,
			Ref:   ".bytemind/skills/review",
		},
		Status:       ExtensionStatusReady,
		Capabilities: CapabilitySet{Prompts: 1, Tools: 2},
	}
	if !valid.Valid() {
		t.Fatal("expected extension info to be valid")
	}

	cases := []ExtensionInfo{
		{Name: "review", Kind: ExtensionSkill},
		{ID: "skill.review", Kind: ExtensionSkill},
		{ID: "skill.review", Name: "review"},
		{ID: "skill.review", Name: "review", Kind: ExtensionKind("unknown")},
		{ID: "skill.review", Name: "review", Kind: ExtensionSkill, Source: ExtensionSource{Ref: ".bytemind/skills/review"}},
		{ID: "skill.review", Name: "review", Kind: ExtensionSkill, Source: ExtensionSource{Scope: ExtensionScopeProject}},
		{ID: "skill.review", Name: "review", Kind: ExtensionSkill, Source: ExtensionSource{Scope: ExtensionScope("bad"), Ref: ".bytemind/skills/review"}},
	}
	for _, tc := range cases {
		if tc.Valid() {
			t.Fatalf("expected invalid extension info: %+v", tc)
		}
	}
}

func TestExtensionInfoIsZero(t *testing.T) {
	if !((ExtensionInfo{}).IsZero()) {
		t.Fatal("expected zero extension info")
	}

	cases := []ExtensionInfo{
		{ID: "skill.review"},
		{Version: "1.0.0"},
		{Title: "Review"},
		{Description: "desc"},
		{Source: ExtensionSource{Scope: ExtensionScopeProject}},
		{Source: ExtensionSource{Ref: ".bytemind/skills/review"}},
		{Capabilities: CapabilitySet{Tools: 1}},
		{Manifest: Manifest{Name: "review"}},
		{Manifest: Manifest{Kind: ExtensionSkill}},
		{Manifest: Manifest{Source: ExtensionSource{Ref: "manifest.json"}}},
		{Health: HealthSnapshot{Status: ExtensionStatusReady}},
		{Health: HealthSnapshot{Message: "ok"}},
		{Health: HealthSnapshot{LastError: ErrCodeLoadFailed}},
		{Health: HealthSnapshot{CheckedAtUTC: "2026-04-17T00:00:00Z"}},
		{Status: ExtensionStatusReady},
	}
	for _, tc := range cases {
		if tc.IsZero() {
			t.Fatalf("expected non-zero extension info: %+v", tc)
		}
	}
}

func TestExtensionErrorWrap(t *testing.T) {
	err := wrapError(ErrCodeLoadFailed, "load extension", nil)
	extErr, ok := err.(*ExtensionError)
	if !ok {
		t.Fatalf("expected ExtensionError, got %T", err)
	}
	if extErr.Code != ErrCodeLoadFailed {
		t.Fatalf("unexpected code: %s", extErr.Code)
	}
	if extErr.Message == "" {
		t.Fatal("expected message")
	}
	if extErr.Unwrap() != nil {
		t.Fatal("expected nil unwrap")
	}
	if extErr.CodeString() != string(ErrCodeLoadFailed) {
		t.Fatalf("unexpected code string: %q", extErr.CodeString())
	}
}

func TestExtensionErrorWithCause(t *testing.T) {
	cause := errors.New("boom")
	err := wrapError(ErrCodeUnloadFailed, "unload extension", cause)
	extErr, ok := err.(*ExtensionError)
	if !ok {
		t.Fatalf("expected ExtensionError, got %T", err)
	}
	if !errors.Is(extErr, cause) {
		t.Fatal("expected wrapped cause")
	}
	if extErr.Error() == "" {
		t.Fatal("expected error string")
	}
}

func TestNilExtensionErrorBehaviors(t *testing.T) {
	var err *ExtensionError
	if err.Error() != "" {
		t.Fatalf("expected empty error string, got %q", err.Error())
	}
	if err.Unwrap() != nil {
		t.Fatal("expected nil unwrap")
	}
	if err.CodeString() != "" {
		t.Fatalf("expected empty code string, got %q", err.CodeString())
	}
}
