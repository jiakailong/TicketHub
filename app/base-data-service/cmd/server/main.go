package main

import (
	"context"
	"flag"
	"log"
	"net/http"

	kgrpc "github.com/go-kratos/kratos/v2/transport/grpc"
	khttp "github.com/go-kratos/kratos/v2/transport/http"

	basev1 "tickethub/api/proto/base/v1"
	baseapp "tickethub/app/base-data-service/internal/application"
	"tickethub/app/base-data-service/internal/infrastructure/memory"
	basemysql "tickethub/app/base-data-service/internal/infrastructure/mysql"
	basegrpc "tickethub/app/base-data-service/internal/interfaces/grpc"
	basehttp "tickethub/app/base-data-service/internal/interfaces/http"
	"tickethub/pkg/bootstrap"
	"tickethub/pkg/config"
	"tickethub/pkg/db"
)

func main() {
	conf := flag.String("conf", "app/base-data-service/configs/config.yaml", "config file path")
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
		basev1.RegisterBaseDataServiceServer(server, grpcServer)
	}); err != nil {
		log.Fatal(err)
	}
}

func registerHTTP(server *khttp.Server) {
	server.HandleFunc("/v1/base/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"service":"base-data-service","status":"ok"}`))
	})
}

func buildHandlers(cfg config.Config) (basehttp.Handler, basegrpc.Server, error) {
	var repo baseapp.BaseRepository
	if cfg.UseInfrastructure() {
		mysqlDB, err := db.OpenMySQL(context.Background(), cfg.MySQL)
		if err != nil {
			return basehttp.Handler{}, basegrpc.Server{}, err
		}
		repo = basemysql.NewBaseRepository(mysqlDB)
	} else {
		repo = memory.NewBaseRepository()
	}
	service := baseapp.NewBaseQueryService(repo)
	return basehttp.NewHandler(service), basegrpc.NewServer(service), nil
}
