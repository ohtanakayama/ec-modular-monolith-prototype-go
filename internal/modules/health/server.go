// Package health implements the internal health.v1.HealthService.
// Distinct from grpc.health.v1.Health (which we also register for ops tooling
// like Kubernetes probes): this one demonstrates the proto-first → gRPC round-trip
// end-to-end on the same server, with no DB dependency in Step 0.
package health

import (
	"context"

	healthv1 "github.com/ohtanakayama/ec-modular-monolith-prototype-go/gen/proto/health/v1"
)

type Server struct {
	healthv1.UnimplementedHealthServiceServer
}

func NewServer() *Server { return &Server{} }

func (s *Server) Ping(ctx context.Context, _ *healthv1.PingRequest) (*healthv1.PingResponse, error) {
	return &healthv1.PingResponse{Status: "ok"}, nil
}
