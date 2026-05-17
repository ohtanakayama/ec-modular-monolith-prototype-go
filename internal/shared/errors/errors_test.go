package errors_test

import (
	"errors"
	"fmt"
	"testing"

	derrors "github.com/ohtanakayama/ec-modular-monolith-prototype-go/internal/shared/errors"
)

func TestValidationError_AsExtractsType(t *testing.T) {
	wrapped := fmt.Errorf("usecase failed: %w", derrors.NewValidationError("email", "invalid format"))

	var ve *derrors.ValidationError
	if !errors.As(wrapped, &ve) {
		t.Fatalf("errors.As did not match ValidationError; got %v", wrapped)
	}
	if ve.Field != "email" || ve.Message != "invalid format" {
		t.Fatalf("unexpected fields: %+v", ve)
	}
}

func TestNotFoundError_Message(t *testing.T) {
	err := derrors.NewNotFoundError("member", "abc-123")
	want := "not found: member id=abc-123"
	if err.Error() != want {
		t.Fatalf("got %q, want %q", err.Error(), want)
	}
}

func TestConflictError_AsThroughWrap(t *testing.T) {
	wrapped := fmt.Errorf("register: %w", derrors.NewConflictError("member", "email already used"))

	var ce *derrors.ConflictError
	if !errors.As(wrapped, &ce) {
		t.Fatalf("errors.As did not match ConflictError")
	}
	if ce.Resource != "member" {
		t.Fatalf("unexpected resource: %q", ce.Resource)
	}
}

func TestDomainError_CodeOptional(t *testing.T) {
	with := derrors.NewDomainError("insufficient_stock", "qty=0")
	if with.Error() != "domain[insufficient_stock]: qty=0" {
		t.Fatalf("got %q", with.Error())
	}
	without := derrors.NewDomainError("", "broken invariant")
	if without.Error() != "domain: broken invariant" {
		t.Fatalf("got %q", without.Error())
	}
}

func TestErrorTypes_AreDistinct(t *testing.T) {
	ve := derrors.NewValidationError("x", "y")
	var ne *derrors.NotFoundError
	if errors.As(ve, &ne) {
		t.Fatalf("ValidationError must not match NotFoundError via errors.As")
	}
}
