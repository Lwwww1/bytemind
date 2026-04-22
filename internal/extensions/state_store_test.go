package extensions

import "testing"

func TestStateStoreBeginLoadBusy(t *testing.T) {
	store := newStateStore()
	if err := store.beginLoad("skill.review"); err != nil {
		t.Fatalf("beginLoad failed: %v", err)
	}
	if err := store.beginLoad("skill.review"); err == nil {
		t.Fatal("expected busy error")
	}
}

func TestStateStoreBeginLoadAlreadyLoaded(t *testing.T) {
	store := newStateStore()
	store.set(ExtensionInfo{ID: "skill.review", Name: "review", Kind: ExtensionSkill, Source: ExtensionSource{Scope: ExtensionScopeProject, Ref: "x"}, Status: ExtensionStatusActive})
	if err := store.beginLoad("skill.review"); err == nil {
		t.Fatal("expected already loaded error")
	}
}

func TestStateStoreCancelLoadClearsLoading(t *testing.T) {
	store := newStateStore()
	if err := store.beginLoad("skill.review"); err != nil {
		t.Fatalf("beginLoad failed: %v", err)
	}
	store.cancelLoad("skill.review")
	if err := store.beginLoad("skill.review"); err != nil {
		t.Fatalf("expected beginLoad to succeed after cancel, got %v", err)
	}
}

func TestStateStoreDeleteCollectsIdleLock(t *testing.T) {
	store := newStateStore()
	if err := store.withLock("skill.review", func() error {
		store.set(ExtensionInfo{ID: "skill.review", Name: "review", Kind: ExtensionSkill, Source: ExtensionSource{Scope: ExtensionScopeProject, Ref: "x"}, Status: ExtensionStatusActive})
		return nil
	}); err != nil {
		t.Fatalf("withLock failed: %v", err)
	}
	if _, ok := store.locks["skill.review"]; !ok {
		t.Fatal("expected lock to exist before delete")
	}
	store.delete("skill.review")
	if _, ok := store.locks["skill.review"]; ok {
		t.Fatal("expected idle lock to be garbage collected")
	}
}
