package bootstrap

import (
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/transport"
	kgrpc "github.com/go-kratos/kratos/v2/transport/grpc"
	khttp "github.com/go-kratos/kratos/v2/transport/http"

	"tickethub/pkg/config"
	therrors "tickethub/pkg/errors"
	"tickethub/pkg/observability"
)

type RegisterHTTP func(*khttp.Server)
type RegisterGRPC func(*kgrpc.Server)

func RunHTTP(cfg config.Config, register RegisterHTTP) error {
	return RunHTTPAndGRPC(cfg, register, nil)
}

func RunHTTPAndGRPC(cfg config.Config, registerHTTP RegisterHTTP, registerGRPC RegisterGRPC) error {
	observability.ConfigureMetrics(cfg.Service.Name)
	logger := log.With(log.NewStdLogger(os.Stdout),
		"ts", log.DefaultTimestamp,
		"caller", log.DefaultCaller,
		"service", cfg.Service.Name,
	)
	server := khttp.NewServer(
		khttp.Address(cfg.HTTP.Addr),
		khttp.Timeout(cfg.HTTP.TimeoutDuration(5*time.Second)),
		khttp.Middleware(observability.ServerMiddleware()),
	)
	registerSystemRoutes(server, cfg)
	if registerHTTP != nil {
		registerHTTP(server)
	}
	servers := []transport.Server{server}
	if registerGRPC != nil && cfg.GRPC.Addr != "" {
		grpcServer := kgrpc.NewServer(
			kgrpc.Address(cfg.GRPC.Addr),
			kgrpc.Timeout(cfg.GRPC.TimeoutDuration(5*time.Second)),
			kgrpc.Middleware(observability.ServerMiddleware(), therrors.GRPCServerMiddleware()),
		)
		registerGRPC(grpcServer)
		servers = append(servers, grpcServer)
	}
	app := kratos.New(
		kratos.Name(cfg.Service.Name),
		kratos.Logger(logger),
		kratos.Server(servers...),
	)
	return app.Run()
}

func registerSystemRoutes(server *khttp.Server, cfg config.Config) {
	server.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"service": cfg.Service.Name,
			"status":  "ok",
		})
	})
	server.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"service": cfg.Service.Name,
			"ready":   true,
		})
	})
	server.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_ = observability.WritePrometheus(w)
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
