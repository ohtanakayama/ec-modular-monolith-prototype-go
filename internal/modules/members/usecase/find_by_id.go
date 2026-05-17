package usecase

import (
	"context"

	"github.com/ohtanakayama/ec-modular-monolith-prototype-go/internal/modules/members/domain/member"
)

type FindByIDFunc func(context.Context, string) (*member.Member, error)

// NewFindByID is currently a thin pass-through to the repository; the seam
// exists so future cross-cutting concerns (e.g., access control, caching)
// can be added without touching handlers or the repository contract.
func NewFindByID(repo member.MemberRepository) FindByIDFunc {
	return func(ctx context.Context, id string) (*member.Member, error) {
		return repo.FindByID(ctx, id)
	}
}
