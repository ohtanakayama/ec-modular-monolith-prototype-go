package membersgrpc

import (
	"errors"
	"fmt"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	derrors "github.com/ohtanakayama/ec-modular-monolith-prototype-go/internal/shared/errors"
)

func TestToGrpcStatus_Mapping(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want codes.Code
	}{
		{"validation", derrors.NewValidationError("email", "bad"), codes.InvalidArgument},
		{"not found", derrors.NewNotFoundError("member", "x"), codes.NotFound},
		{"conflict", derrors.NewConflictError("member", "dup"), codes.AlreadyExists},
		{"domain", derrors.NewDomainError("rule", "violated"), codes.FailedPrecondition},
		{"unknown", errors.New("boom"), codes.Internal},
		{"wrapped validation", fmt.Errorf("ctx: %w", derrors.NewValidationError("x", "y")), codes.InvalidArgument},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := toGrpcStatus(c.err)
			st, ok := status.FromError(got)
			if !ok {
				t.Fatalf("not a status error: %v", got)
			}
			if st.Code() != c.want {
				t.Fatalf("code = %v, want %v", st.Code(), c.want)
			}
		})
	}
}

func TestToGrpcStatus_NilPassthrough(t *testing.T) {
	if got := toGrpcStatus(nil); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}
