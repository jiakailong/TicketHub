package main

import (
	"context"
	"flag"
	"log"
	"net/http"

	kgrpc "github.com/go-kratos/kratos/v2/transport/grpc"
	khttp "github.com/go-kratos/kratos/v2/transport/http"

	customizev1 "tickethub/api/proto/customize/v1"
	customizeapp "tickethub/app/customize-service/internal/application"
	"tickethub/app/customize-service/internal/infrastructure/memory"
	customizemysql "tickethub/app/customize-service/internal/infrastructure/mysql"
	customizegrpc "tickethub/app/customize-service/internal/interfaces/grpc"
	customizehttp "tickethub/app/customize-service/internal/interfaces/http"
	"tickethub/pkg/bootstrap"
	"tickethub/pkg/config"
	"tickethub/pkg/db"
)

func main() {
	conf := flag.String("conf", "app/customize-service/configs/config.yaml", "config file path")
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
		customizev1.RegisterCustomizeServiceServer(server, grpcServer)
	}); err != nil {
		log.Fatal(err)
	}
}

func registerHTTP(server *khttp.Server) {
	server.HandleFunc("/v1/customize/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"service":"customize-service","status":"ok"}`))
	})
}

func buildHandlers(cfg config.Config) (customizehttp.Handler, customizegrpc.Server, error) {
	var repo customizeapp.MessageRecordRepository
	if cfg.UseInfrastructure() {
		mysqlDB, err := db.OpenMySQL(context.Background(), cfg.MySQL)
		if err != nil {
			return customizehttp.Handler{}, customizegrpc.Server{}, err
		}
		repo = customizemysql.NewMessageRecordRepository(mysqlDB)
	} else {
		repo = memory.NewMessageRecordRepository()
	}
	service := customizeapp.NewMessageRecordService(repo)
	return customizehttp.NewHandler(service), customizegrpc.NewServer(service), nil
}
