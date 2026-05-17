package tx

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
)

// txBeginner is the subset of pgxpool.Pool that this interceptor relies on.
// Decoupling here keeps unit tests pool-free.
type txBeginner interface {
	BeginTx(ctx context.Context, opts pgx.TxOptions) (pgx.Tx, error)
}

// UnaryTxInterceptor wraps every unary RPC in a single Postgres transaction.
// On handler success the tx is committed; on error or panic it is rolled back.
// The tx is stashed on ctx via WithTx and retrieved by repositories with GetTx.
//
// Streaming RPCs are out of scope for Step 0+1 (see ADR 0008).
func UnaryTxInterceptor(pool *pgxpool.Pool) grpc.UnaryServerInterceptor {
	return unaryTxInterceptor(pool)
}

func unaryTxInterceptor(b txBeginner) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, h grpc.UnaryHandler) (resp any, err error) {
		tx, err := b.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return nil, err
		}

		committed := false
		defer func() {
			if !committed {
				// Use context.Background() so rollback survives a cancelled ctx.
				_ = tx.Rollback(context.Background())
			}
		}()

		resp, err = h(WithTx(ctx, tx), req)
		if err != nil {
			return nil, err
		}

		if cerr := tx.Commit(ctx); cerr != nil {
			return nil, cerr
		}
		committed = true
		return resp, nil
	}
}
