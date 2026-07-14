package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"time"

	kgrpc "github.com/go-kratos/kratos/v2/transport/grpc"
	khttp "github.com/go-kratos/kratos/v2/transport/http"

	payv1 "tickethub/api/proto/pay/v1"
	payapp "tickethub/app/pay-service/internal/application"
	"tickethub/app/pay-service/internal/infrastructure/gateway"
	"tickethub/app/pay-service/internal/infrastructure/memory"
	paymysql "tickethub/app/pay-service/internal/infrastructure/mysql"
	"tickethub/app/pay-service/internal/infrastructure/rpc"
	paygrpc "tickethub/app/pay-service/internal/interfaces/grpc"
	payhttp "tickethub/app/pay-service/internal/interfaces/http"
	"tickethub/pkg/bootstrap"
	"tickethub/pkg/config"
	"tickethub/pkg/db"
	"tickethub/pkg/lock"
	thredis "tickethub/pkg/redis"
)

func main() {
	conf := flag.String("conf", "app/pay-service/configs/config.yaml", "config file path")
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
		payv1.RegisterPayServiceServer(server, grpcServer)
	}); err != nil {
		log.Fatal(err)
	}
}

func registerHTTP(server *khttp.Server) {
	server.HandleFunc("/v1/pay/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"service":"pay-service","status":"ok"}`))
	})
}

func buildHandlers(cfg config.Config) (payhttp.Handler, paygrpc.Server, error) {
	var repo payapp.PaymentRepository
	var orderClient payapp.OrderPaymentClient
	if cfg.UseInfrastructure() {
		mysqlDB, err := db.OpenMySQL(context.Background(), cfg.MySQL)
		if err != nil {
			return payhttp.Handler{}, paygrpc.Server{}, err
		}
		repo = paymysql.NewPaymentRepository(mysqlDB)
		client, err := rpc.NewOrderPaymentClient(cfg.GRPCUpstreams["order-service"])
		if err != nil {
			return payhttp.Handler{}, paygrpc.Server{}, err
		}
		orderClient = client
	} else {
		repo = memory.NewPaymentRepository()
	}
	service := payapp.NewPayUsecase(repo, gateway.NewMockGateway(""), orderClient)
	if cfg.UseInfrastructure() {
		service = service.WithLocker(lock.NewRedisLocker(thredis.NewClient(cfg.Redis)), 10*time.Second)
	}
	return payhttp.NewHandler(service), paygrpc.NewServer(service), nil
}
