// Package usecase is the application layer for the members BC.
// Usecases are exposed as function types via currying (ADR 0003).
package usecase

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/ohtanakayama/ec-modular-monolith-prototype-go/internal/modules/members/domain/member"
)

type RegisterInput struct {
	Email string
	Name  string
}

type RegisterFunc func(context.Context, RegisterInput) (*member.Member, error)

// NewRegister wires the repository into the Register usecase.
// Time and id source are taken from the wall clock / google/uuid; swap
// callers in tests by stubbing the repository, not by injecting clocks
// (asserting on Email/Name fields is enough at this scope — see register_test.go).
func NewRegister(repo member.MemberRepository) RegisterFunc {
	return func(ctx context.Context, in RegisterInput) (*member.Member, error) {
		email, err := member.NewEmail(in.Email)
		if err != nil {
			return nil, err
		}
		name, err := member.NewMemberName(in.Name)
		if err != nil {
			return nil, err
		}
		m := member.New(uuid.NewString(), email, name, time.Now().UTC())
		if err := repo.Save(ctx, m); err != nil {
			return nil, err
		}
		return m, nil
	}
}
