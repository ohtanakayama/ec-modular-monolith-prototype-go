package usecase_test

import (
	"context"
	"errors"
	"testing"

	derrors "github.com/ohtanakayama/ec-modular-monolith-prototype-go/internal/shared/errors"
	"github.com/ohtanakayama/ec-modular-monolith-prototype-go/internal/modules/members/domain/member"
	"github.com/ohtanakayama/ec-modular-monolith-prototype-go/internal/modules/members/usecase"
)

type stubRepo struct {
	saved     *member.Member
	saveErr   error
	findRes   *member.Member
	findErr   error
	saveCalls int
	findCalls int
}

func (s *stubRepo) Save(_ context.Context, m *member.Member) error {
	s.saveCalls++
	if s.saveErr != nil {
		return s.saveErr
	}
	s.saved = m
	return nil
}

func (s *stubRepo) FindByID(_ context.Context, _ string) (*member.Member, error) {
	s.findCalls++
	return s.findRes, s.findErr
}

func TestRegister_Success_SavesAndReturnsHydratedMember(t *testing.T) {
	repo := &stubRepo{}
	register := usecase.NewRegister(repo)

	got, err := register(context.Background(), usecase.RegisterInput{
		Email: "alice@example.com",
		Name:  "Alice",
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.ID() == "" {
		t.Fatalf("expected non-empty id, got empty")
	}
	if got.Email().String() != "alice@example.com" {
		t.Fatalf("email = %q, want %q", got.Email().String(), "alice@example.com")
	}
	if got.Name().String() != "Alice" {
		t.Fatalf("name = %q, want %q", got.Name().String(), "Alice")
	}
	if got.CreatedAt().IsZero() {
		t.Fatalf("createdAt must be set")
	}
	if repo.saveCalls != 1 {
		t.Fatalf("save called %d times, want 1", repo.saveCalls)
	}
	if repo.saved.ID() != got.ID() {
		t.Fatalf("saved member id mismatches returned id")
	}
}

func TestRegister_InvalidEmail_ReturnsValidationErrorAndDoesNotSave(t *testing.T) {
	repo := &stubRepo{}
	register := usecase.NewRegister(repo)

	_, err := register(context.Background(), usecase.RegisterInput{
		Email: "not-an-email",
		Name:  "Alice",
	})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	var ve *derrors.ValidationError
	if !errors.As(err, &ve) || ve.Field != "email" {
		t.Fatalf("expected ValidationError(email), got %v", err)
	}
	if repo.saveCalls != 0 {
		t.Fatalf("save should not be called on validation failure")
	}
}

func TestRegister_InvalidName_ReturnsValidationErrorAndDoesNotSave(t *testing.T) {
	repo := &stubRepo{}
	register := usecase.NewRegister(repo)

	_, err := register(context.Background(), usecase.RegisterInput{
		Email: "alice@example.com",
		Name:  "",
	})
	var ve *derrors.ValidationError
	if !errors.As(err, &ve) || ve.Field != "name" {
		t.Fatalf("expected ValidationError(name), got %v", err)
	}
	if repo.saveCalls != 0 {
		t.Fatalf("save should not be called on validation failure")
	}
}

func TestRegister_RepoConflict_Propagates(t *testing.T) {
	conflict := derrors.NewConflictError("member", "email already used")
	repo := &stubRepo{saveErr: conflict}
	register := usecase.NewRegister(repo)

	_, err := register(context.Background(), usecase.RegisterInput{
		Email: "alice@example.com",
		Name:  "Alice",
	})
	var ce *derrors.ConflictError
	if !errors.As(err, &ce) {
		t.Fatalf("expected ConflictError, got %v", err)
	}
}

func TestFindByID_NotFound_Propagates(t *testing.T) {
	nf := derrors.NewNotFoundError("member", "abc")
	repo := &stubRepo{findErr: nf}
	find := usecase.NewFindByID(repo)

	_, err := find(context.Background(), "abc")
	var nferr *derrors.NotFoundError
	if !errors.As(err, &nferr) {
		t.Fatalf("expected NotFoundError, got %v", err)
	}
}
