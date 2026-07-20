package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	kgrpc "github.com/go-kratos/kratos/v2/transport/grpc"
	khttp "github.com/go-kratos/kratos/v2/transport/http"

	orderv1 "tickethub/api/proto/order/v1"
	orderapp "tickethub/app/order-service/internal/application"
	orderdomain "tickethub/app/order-service/internal/domain/order"
	"tickethub/app/order-service/internal/infrastructure/memory"
	ordermysql "tickethub/app/order-service/internal/infrastructure/mysql"
	"tickethub/app/order-service/internal/infrastructure/rpc"
	ordergrpc "tickethub/app/order-service/internal/interfaces/grpc"
	orderhttp "tickethub/app/order-service/internal/interfaces/http"
	orderworker "tickethub/app/order-service/internal/interfaces/worker"
	"tickethub/pkg/bootstrap"
	"tickethub/pkg/config"
	"tickethub/pkg/db"
	"tickethub/pkg/delayqueue"
	"tickethub/pkg/lock"
	"tickethub/pkg/mq"
	thredis "tickethub/pkg/redis"
	"tickethub/pkg/sharding"
)

func main() {
	conf := flag.String("conf", "app/order-service/configs/config.yaml", "config file path")
	flag.Parse()

	cfg, err := config.Load(*conf)
	if err != nil {
		log.Fatal(err)
	}
	httpHandler, grpcServer, runner, err := buildHandlers(cfg)
	if err != nil {
		log.Fatal(err)
	}
	runner.Start(context.Background())
	if err := bootstrap.RunHTTPAndGRPC(cfg, func(server *khttp.Server) {
		registerHTTP(server)
		httpHandler.Register(server)
	}, func(server *kgrpc.Server) {
		orderv1.RegisterOrderServiceServer(server, grpcServer)
	}); err != nil {
		log.Fatal(err)
	}
}

func registerHTTP(server *khttp.Server) {
	server.HandleFunc("/v1/order/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"service":"order-service","status":"ok"}`))
	})
}

func buildHandlers(cfg config.Config) (orderhttp.Handler, ordergrpc.Server, orderworker.Runner, error) {
	var ordersRepo orderdomain.Repository
	var discards orderapp.DiscardOrderCompensationRepository
	var reconciliations orderapp.ReconciliationRepository
	var publisher mq.Producer
	var consumer mq.Consumer
	var cancelQueue delayqueue.Queue
	var mappingRefresher orderworker.ShardMappingRefresher
	if cfg.UseInfrastructure() {
		mysqlDB, err := db.OpenMySQL(context.Background(), cfg.MySQL)
		if err != nil {
			return orderhttp.Handler{}, ordergrpc.Server{}, orderworker.Runner{}, err
		}
		discards = ordermysql.NewDiscardRepository(mysqlDB)
		if cfg.Sharding.Enabled {
			shardPool, err := db.OpenMySQLShardPool(context.Background(), cfg.Sharding)
			if err != nil {
				return orderhttp.Handler{}, ordergrpc.Server{}, orderworker.Runner{}, err
			}
			fallbackRouter := sharding.NewGeneOrderRouter(
				cfg.Sharding.DatabasePrefix,
				cfg.Sharding.TablePrefix,
				cfg.Sharding.DatabaseCount,
				cfg.Sharding.TableCount,
			)
			router := sharding.NewMappingOrderRouter(fallbackRouter)
			if addr := cfg.GRPCUpstreams["migrate-service"]; addr != "" {
				mappingClient, err := rpc.NewMigrationGRPCClient(addr)
				if err != nil {
					return orderhttp.Handler{}, ordergrpc.Server{}, orderworker.Runner{}, err
				}
				refresher := orderapp.NewShardMappingRefreshService(mappingClient, router)
				refreshContext, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				err = refresher.Refresh(refreshContext)
				cancel()
				if err != nil {
					return orderhttp.Handler{}, ordergrpc.Server{}, orderworker.Runner{}, fmt.Errorf("load initial shard mappings: %w", err)
				}
				mappingRefresher = refresher
			} else {
				return orderhttp.Handler{}, ordergrpc.Server{}, orderworker.Runner{}, fmt.Errorf("grpc_upstreams.migrate-service is required when sharding is enabled")
			}
			shardedOrders := ordermysql.NewShardedOrderRepository(shardPool, router)
			ordersRepo = shardedOrders
			reconciliations = ordermysql.NewShardedReconciliationRepository(mysqlDB, shardedOrders)
		} else {
			ordersRepo = ordermysql.NewOrderRepository(mysqlDB)
			reconciliations = ordermysql.NewReconciliationRepository(mysqlDB)
		}
		publisher = mq.NewKafkaProducer(cfg.Kafka.Brokers, cfg.Kafka.ClientID)
		consumer = mq.NewKafkaConsumer(cfg.Kafka.Brokers, cfg.Kafka.GroupID)
		redisClient := thredis.NewClient(cfg.Redis)
		cancelQueue = delayqueue.NewRedisQueue(redisClient, cfg.Redis.KeyPrefix+":delayqueue").WithVisibilityTimeout(cfg.Workers.DelayVisibilityDuration())
	} else {
		ordersRepo = memory.NewOrderRepository()
		discards = memory.NewDiscardRepository()
		reconciliations = memory.NewReconciliationRepository()
		broker := mq.NewMemoryBroker()
		publisher = broker
		consumer = broker
		cancelQueue = delayqueue.NewMemoryQueue()
	}
	programClient := rpc.NewProgramClient()
	if addr := cfg.GRPCUpstreams["program-service"]; addr != "" {
		client, err := rpc.NewProgramGRPCClient(addr)
		if err != nil {
			return orderhttp.Handler{}, ordergrpc.Server{}, orderworker.Runner{}, err
		}
		programClient = client
	}
	service := orderapp.NewOrderCommandService(ordersRepo, programClient)
	if cfg.UseInfrastructure() {
		service = service.WithLocker(lock.NewRedisLocker(thredis.NewClient(cfg.Redis)), 5*time.Second)
	}
	queries := orderapp.NewOrderQueryService(ordersRepo)
	compensation := orderapp.NewDiscardOrderCompensationService(discards, publisher, cfg.Kafka.Topics["create_order"]).WithInventory(programClient)
	usageRepo, ok := ordersRepo.(orderapp.InventoryUsageRepository)
	if !ok {
		return orderhttp.Handler{}, ordergrpc.Server{}, orderworker.Runner{}, fmt.Errorf("order repository does not support inventory usage aggregation")
	}
	reconciliation := orderapp.NewReconciliationService(reconciliations).WithInventory(usageRepo, programClient)
	createConsumer := orderapp.NewCreateOrderConsumer(ordersRepo, discards, 5*time.Minute).
		WithCancelDelayQueue(cancelQueue, cfg.Workers.CancelDelayDuration()).
		WithInventoryRollback(programClient)
	cancelWorker := orderapp.NewCancelOrderWorker(cancelQueue, service)
	runner := orderworker.NewRunner(consumer, cfg.Kafka.Topics["create_order"], createConsumer, cancelWorker).
		WithSettings(
			cfg.Workers.PollIntervalDuration(),
			cfg.Workers.CreateBatchSize,
			cfg.Workers.CancelBatchSize,
			cfg.Workers.Enabled("create_order"),
			cfg.Workers.Enabled("cancel_order"),
		)
	if mappingRefresher != nil {
		runner = runner.WithShardMappingRefresher(mappingRefresher, 5*time.Second)
	}
	return orderhttp.NewHandler(service, queries, compensation, reconciliation), ordergrpc.NewServer(ordersRepo, service, queries, reconciliation), runner, nil
}
