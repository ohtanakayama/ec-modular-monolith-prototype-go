package membersgrpc

import (
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	derrors "github.com/ohtanakayama/ec-modular-monolith-prototype-go/internal/shared/errors"
)

// toGrpcStatus maps domain-typed errors to gRPC status errors per ADR 0009.
// Unknown errors degrade to codes.Internal so transports never leak nil.
func toGrpcStatus(err error) error {
	if err == nil {
		return nil
	}
	var ve *derrors.ValidationError
	if errors.As(err, &ve) {
		return status.Error(codes.InvalidArgument, ve.Error())
	}
	var nf *derrors.NotFoundError
	if errors.As(err, &nf) {
		return status.Error(codes.NotFound, nf.Error())
	}
	var conf *derrors.ConflictError
	if errors.As(err, &conf) {
		return status.Error(codes.AlreadyExists, conf.Error())
	}
	var de *derrors.DomainError
	if errors.As(err, &de) {
		return status.Error(codes.FailedPrecondition, de.Error())
	}
	return status.Error(codes.Internal, err.Error())
}
