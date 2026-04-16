package storage

import (
	"errors"
	"strings"
	"testing"
)

func TestLockerErrorMethods(t *testing.T) {
	cause := errors.New("boom")
	err := newLockerError(ErrCodeLockTimeout, "lock timeout", true, cause)

	var semantic interface {
		error
		Code() string
		Retryable() bool
		Unwrap() error
	}
	if !errors.As(err, &semantic) {
		t.Fatalf("expected semantic locker error, got %T", err)
	}

	if !strings.Contains(semantic.Error(), "lock timeout") || !strings.Contains(semantic.Error(), "boom") {
		t.Fatalf("unexpected error string: %q", semantic.Error())
	}
	if semantic.Code() != string(ErrCodeLockTimeout) {
		t.Fatalf("unexpected error code: %q", semantic.Code())
	}
	if !semantic.Retryable() {
		t.Fatal("expected retryable locker error")
	}
	if !errors.Is(err, cause) {
		t.Fatalf("expected wrapped cause %v in %v", cause, err)
	}
}

func TestLockerErrorNilReceiver(t *testing.T) {
	var lockerErr *lockerError
	if lockerErr.Error() != "" {
		t.Fatalf("expected empty string from nil Error(), got %q", lockerErr.Error())
	}
	if lockerErr.Code() != "" {
		t.Fatalf("expected empty code from nil Code(), got %q", lockerErr.Code())
	}
	if lockerErr.Retryable() {
		t.Fatal("expected nil Retryable() to be false")
	}
	if lockerErr.Unwrap() != nil {
		t.Fatal("expected nil Unwrap() from nil receiver")
	}
}

func TestHasErrorCode(t *testing.T) {
	if hasErrorCode(errors.New("plain"), ErrCodeLockTimeout) {
		t.Fatal("expected plain errors to have no semantic code")
	}
	if !hasErrorCode(newLockerError(ErrCodeLockTimeout, "timeout", true, nil), ErrCodeLockTimeout) {
		t.Fatal("expected locker error to match semantic code")
	}
}
