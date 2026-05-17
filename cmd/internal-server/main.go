package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kelseyhightower/envconfig"
	"google.golang.org/grpc"
	grpchealth "google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	healthv1 "github.com/ohtanakayama/ec-modular-monolith-prototype-go/gen/proto/health/v1"
	"github.com/ohtanakayama/ec-modular-monolith-prototype-go/internal/modules/health"
	"github.com/ohtanakayama/ec-modular-monolith-prototype-go/internal/shared/tx"
)

type Config struct {
	DatabaseURL string `envconfig:"DATABASE_URL" required:"true"`
	GRPCPort    string `envconfig:"GRPC_PORT" default:"9090"`
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		logger.Error("invalid config", slog.String("err", err.Error()))
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("pgxpool.New failed", slog.String("err", err.Error()))
		os.Exit(1)
	}
	defer pool.Close()

	s := grpc.NewServer(
		grpc.UnaryInterceptor(tx.UnaryTxInterceptor(pool)),
	)

	// Internal proto-first health service (own contract).
	healthv1.RegisterHealthServiceServer(s, health.NewServer())

	// Standard grpc.health.v1.Health for ops tooling (k8s liveness probes etc.).
	healthpb.RegisterHealthServer(s, grpchealth.NewServer())

	reflection.Register(s)

	lis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		logger.Error("listen failed", slog.String("err", err.Error()))
		os.Exit(1)
	}

	go func() {
		<-ctx.Done()
		logger.Info("shutdown signal received, draining gRPC")
		s.GracefulStop()
	}()

	logger.Info("gRPC server listening", slog.String("port", cfg.GRPCPort))
	if err := s.Serve(lis); err != nil {
		logger.Error("grpc serve failed", slog.String("err", err.Error()))
		os.Exit(1)
	}
}
