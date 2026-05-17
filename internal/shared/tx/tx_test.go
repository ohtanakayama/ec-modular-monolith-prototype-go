package tx

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"google.golang.org/grpc"
)

type fakeTx struct {
	pgx.Tx
	commitCalled   bool
	rollbackCalled bool
	commitErr      error
}

func (f *fakeTx) Commit(context.Context) error {
	f.commitCalled = true
	return f.commitErr
}

func (f *fakeTx) Rollback(context.Context) error {
	f.rollbackCalled = true
	return nil
}

type fakeBeginner struct {
	tx       *fakeTx
	beginErr error
}

func (f *fakeBeginner) BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error) {
	if f.beginErr != nil {
		return nil, f.beginErr
	}
	return f.tx, nil
}

func TestWithTx_GetTx_Roundtrip(t *testing.T) {
	tx := &fakeTx{}
	ctx := WithTx(context.Background(), tx)
	got := GetTx(ctx)
	if got == nil {
		t.Fatalf("GetTx returned nil")
	}
	if got != tx {
		t.Fatalf("GetTx returned different tx")
	}
}

func TestGetTx_NilWhenAbsent(t *testing.T) {
	if got := GetTx(context.Background()); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestUnaryTxInterceptor_CommitOnSuccess(t *testing.T) {
	tx := &fakeTx{}
	icpt := unaryTxInterceptor(&fakeBeginner{tx: tx})

	handler := func(ctx context.Context, req any) (any, error) {
		if got := GetTx(ctx); got == nil {
			t.Fatalf("handler did not receive tx in ctx")
		}
		return "ok", nil
	}

	resp, err := icpt(context.Background(), nil, &grpc.UnaryServerInfo{}, handler)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resp != "ok" {
		t.Fatalf("unexpected resp: %v", resp)
	}
	if !tx.commitCalled {
		t.Fatalf("expected commit, got none")
	}
	if tx.rollbackCalled {
		t.Fatalf("rollback should not be called on success")
	}
}

func TestUnaryTxInterceptor_RollbackOnHandlerError(t *testing.T) {
	tx := &fakeTx{}
	icpt := unaryTxInterceptor(&fakeBeginner{tx: tx})

	wantErr := errors.New("boom")
	handler := func(ctx context.Context, req any) (any, error) {
		return nil, wantErr
	}

	resp, err := icpt(context.Background(), nil, &grpc.UnaryServerInfo{}, handler)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
	if resp != nil {
		t.Fatalf("expected nil resp, got %v", resp)
	}
	if tx.commitCalled {
		t.Fatalf("commit must not run on error")
	}
	if !tx.rollbackCalled {
		t.Fatalf("expected rollback after handler error")
	}
}

func TestUnaryTxInterceptor_BeginError_ShortCircuits(t *testing.T) {
	wantErr := errors.New("begin failed")
	icpt := unaryTxInterceptor(&fakeBeginner{beginErr: wantErr})

	handlerCalled := false
	handler := func(ctx context.Context, req any) (any, error) {
		handlerCalled = true
		return nil, nil
	}

	_, err := icpt(context.Background(), nil, &grpc.UnaryServerInfo{}, handler)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected begin err, got %v", err)
	}
	if handlerCalled {
		t.Fatalf("handler must not run when Begin fails")
	}
}

func TestUnaryTxInterceptor_CommitError_PropagatesAndRollsBack(t *testing.T) {
	commitErr := errors.New("commit failed")
	tx := &fakeTx{commitErr: commitErr}
	icpt := unaryTxInterceptor(&fakeBeginner{tx: tx})

	handler := func(ctx context.Context, req any) (any, error) {
		return "ok", nil
	}

	_, err := icpt(context.Background(), nil, &grpc.UnaryServerInfo{}, handler)
	if !errors.Is(err, commitErr) {
		t.Fatalf("expected commit err, got %v", err)
	}
	if !tx.rollbackCalled {
		t.Fatalf("expected rollback via defer when commit fails")
	}
}
