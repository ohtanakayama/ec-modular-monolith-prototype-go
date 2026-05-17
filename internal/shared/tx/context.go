// Package tx propagates a pgx transaction through context.Context.
// Equivalent in role to AsyncLocalStorage in the TS sibling project.
// Design rationale: ADR 0008.
package tx

import (
	"context"

	"github.com/jackc/pgx/v5"
)

type ctxKey struct{}

// WithTx returns a derived context carrying tx. Use only from this package's
// interceptor; application code should not call this directly.
func WithTx(ctx context.Context, tx pgx.Tx) context.Context {
	return context.WithValue(ctx, ctxKey{}, tx)
}

// GetTx returns the pgx.Tx stored in ctx, or nil when none is set
// (e.g., outside an RPC where the interceptor did not run).
func GetTx(ctx context.Context) pgx.Tx {
	tx, _ := ctx.Value(ctxKey{}).(pgx.Tx)
	return tx
}
