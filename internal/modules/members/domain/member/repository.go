package member

import "context"

// MemberRepository is the port owned by the domain.
// Implementations live in infra and read the active tx from ctx (ADR 0008).
//
// Save persists a Member, returning *ConflictError on duplicate identity / email
// or another error for transport / infra issues.
//
// FindByID returns *NotFoundError when the id is not present.
type MemberRepository interface {
	Save(ctx context.Context, m *Member) error
	FindByID(ctx context.Context, id string) (*Member, error)
}
