// Package membersgrpc adapts the members usecases to the gRPC server.
// It is the only layer aware of the proto-generated types.
package membersgrpc

import (
	"context"

	pb "github.com/ohtanakayama/ec-modular-monolith-prototype-go/gen/proto/members/v1"
	"github.com/ohtanakayama/ec-modular-monolith-prototype-go/internal/modules/members/usecase"
)

type grpcHandler struct {
	pb.UnimplementedMemberServiceServer
	register usecase.RegisterFunc
	findByID usecase.FindByIDFunc
}

func NewHandler(register usecase.RegisterFunc, findByID usecase.FindByIDFunc) pb.MemberServiceServer {
	return &grpcHandler{register: register, findByID: findByID}
}

func (h *grpcHandler) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	m, err := h.register(ctx, usecase.RegisterInput{
		Email: req.GetEmail(),
		Name:  req.GetName(),
	})
	if err != nil {
		return nil, toGrpcStatus(err)
	}
	return &pb.RegisterResponse{Member: toMemberPB(m)}, nil
}

func (h *grpcHandler) GetById(ctx context.Context, req *pb.GetByIdRequest) (*pb.GetByIdResponse, error) {
	m, err := h.findByID(ctx, req.GetId())
	if err != nil {
		return nil, toGrpcStatus(err)
	}
	return &pb.GetByIdResponse{Member: toMemberPB(m)}, nil
}
