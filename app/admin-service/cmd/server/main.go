package main

import (
	"context"
	"flag"
	"log"
	"net/http"

	kgrpc "github.com/go-kratos/kratos/v2/transport/grpc"
	khttp "github.com/go-kratos/kratos/v2/transport/http"

	adminv1 "tickethub/api/proto/admin/v1"
	adminapp "tickethub/app/admin-service/internal/application"
	"tickethub/app/admin-service/internal/infrastructure/memory"
	adminmysql "tickethub/app/admin-service/internal/infrastructure/mysql"
	"tickethub/app/admin-service/internal/infrastructure/rpc"
	admingrpc "tickethub/app/admin-service/internal/interfaces/grpc"
	adminhttp "tickethub/app/admin-service/internal/interfaces/http"
	"tickethub/pkg/bootstrap"
	"tickethub/pkg/config"
	"tickethub/pkg/db"
)

func main() {
	conf := flag.String("conf", "app/admin-service/configs/config.yaml", "config file path")
	flag.Parse()

	cfg, err := config.Load(*conf)
	if err != nil {
		log.Fatal(err)
	}
	httpHandler, grpcServer, err := buildHandlers(cfg)
	if err != nil {
		log.Fatal(err)
	}
	if err := bootstrap.RunHTTPAndGRPC(cfg, func(server *khttp.Server) {
		registerHTTP(server)
		httpHandler.Register(server)
	}, func(server *kgrpc.Server) {
		adminv1.RegisterAdminServiceServer(server, grpcServer)
	}); err != nil {
		log.Fatal(err)
	}
}

func registerHTTP(server *khttp.Server) {
	server.HandleFunc("/v1/admin/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"service":"admin-service","status":"ok"}`))
	})
}

func buildHandlers(cfg config.Config) (adminhttp.Handler, admingrpc.Server, error) {
	var command adminapp.ReconciliationCommand
	if cfg.UseInfrastructure() {
		mysqlDB, err := db.OpenMySQL(context.Background(), cfg.MySQL)
		if err != nil {
			return adminhttp.Handler{}, admingrpc.Server{}, err
		}
		command, err = rpc.NewOrderReconciliationClient(cfg.GRPCUpstreams["order-service"])
		if err != nil {
			return adminhttp.Handler{}, admingrpc.Server{}, err
		}
		query := adminmysql.NewDashboardQuery(mysqlDB)
		return adminhttp.NewHandler(query, command), admingrpc.NewServer(query, command), nil
	}
	query := memory.NewDashboardQuery()
	command = memory.NewReconciliationCommand()
	return adminhttp.NewHandler(query, command), admingrpc.NewServer(query, command), nil
}
