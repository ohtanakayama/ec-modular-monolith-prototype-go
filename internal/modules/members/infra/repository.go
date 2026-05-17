// Package infra holds the Postgres-backed adapters for the members BC.
// The repository implements member.MemberRepository, owns the sqlc Queries,
// and reads the active tx from ctx (ADR 0008).
package infra

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ohtanakayama/ec-modular-monolith-prototype-go/internal/modules/members/domain/member"
	membersdb "github.com/ohtanakayama/ec-modular-monolith-prototype-go/internal/modules/members/infra/db"
	derrors "github.com/ohtanakayama/ec-modular-monolith-prototype-go/internal/shared/errors"
	"github.com/ohtanakayama/ec-modular-monolith-prototype-go/internal/shared/tx"
)

// uniqueViolation is the Postgres SQLSTATE for unique constraint violation.
const uniqueViolation = "23505"

type pgMemberRepository struct{}

// NewMemberRepository constructs a MemberRepository. Implementations are
// stateless: the active transaction is read from ctx per-call, so the same
// repository instance can be shared across all RPCs.
func NewMemberRepository() member.MemberRepository {
	return &pgMemberRepository{}
}

func (r *pgMemberRepository) queries(ctx context.Context) *membersdb.Queries {
	return membersdb.New(tx.GetTx(ctx))
}

func (r *pgMemberRepository) Save(ctx context.Context, m *member.Member) error {
	err := r.queries(ctx).SaveMember(ctx, membersdb.SaveMemberParams{
		ID:    m.ID(),
		Email: m.Email().String(),
		Name:  m.Name().String(),
	})
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return derrors.NewConflictError("member", "email already registered")
	}
	return err
}

func (r *pgMemberRepository) FindByID(ctx context.Context, id string) (*member.Member, error) {
	row, err := r.queries(ctx).FindMemberByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, derrors.NewNotFoundError("member", id)
		}
		return nil, err
	}

	email, err := member.NewEmail(row.Email)
	if err != nil {
		return nil, err
	}
	name, err := member.NewMemberName(row.Name)
	if err != nil {
		return nil, err
	}
	return member.New(row.ID, email, name, row.CreatedAt.Time), nil
}
