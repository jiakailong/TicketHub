package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"time"

	kgrpc "github.com/go-kratos/kratos/v2/transport/grpc"
	khttp "github.com/go-kratos/kratos/v2/transport/http"

	programv1 "tickethub/api/proto/program/v1"
	programapp "tickethub/app/program-service/internal/application"
	programes "tickethub/app/program-service/internal/infrastructure/elasticsearch"
	"tickethub/app/program-service/internal/infrastructure/memory"
	programmysql "tickethub/app/program-service/internal/infrastructure/mysql"
	programredis "tickethub/app/program-service/internal/infrastructure/redis"
	programrpc "tickethub/app/program-service/internal/infrastructure/rpc"
	programgrpc "tickethub/app/program-service/internal/interfaces/grpc"
	programhttp "tickethub/app/program-service/internal/interfaces/http"
	programworker "tickethub/app/program-service/internal/interfaces/worker"
	"tickethub/pkg/bootstrap"
	"tickethub/pkg/cache"
	"tickethub/pkg/config"
	"tickethub/pkg/db"
	"tickethub/pkg/idgen"
	"tickethub/pkg/mq"
	thredis "tickethub/pkg/redis"
)

func main() {
	conf := flag.String("conf", "app/program-service/configs/config.yaml", "config file path")
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
		programv1.RegisterProgramServiceServer(server, grpcServer)
	}); err != nil {
		log.Fatal(err)
	}
}

func registerHTTP(server *khttp.Server) {
	server.HandleFunc("/v1/program/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"service":"program-service","status":"ok"}`))
	})
}

func buildHandlers(cfg config.Config) (programhttp.Handler, programgrpc.Server, programworker.Runner, error) {
	ids, err := idgen.NewSnowflake(2)
	if err != nil {
		return programhttp.Handler{}, programgrpc.Server{}, programworker.Runner{}, err
	}
	var programs programapp.ProgramSearchRepository
	var inventory programapp.InventoryLocker
	var publisher programapp.EventPublisher
	var seatStates programgrpc.SeatStateService
	var inventoryReconciliation programapp.InventoryReconciliationService
	var ticketUsers programapp.TicketUserOwnershipSource
	var idempotency programapp.IdempotencyStore
	var queryPrograms programapp.ProgramSearchRepository
	var purchaseRateLimiter programapp.PurchaseRateLimiter
	var adminPrograms programapp.ProgramAdminService
	var programEvents programworker.ProgramEventRunner
	var indexRunner programworker.IndexSyncRunner
	var outboxRunner programworker.OutboxRunner
	if cfg.UseInfrastructure() {
		mysqlDB, err := db.OpenMySQL(context.Background(), cfg.MySQL)
		if err != nil {
			return programhttp.Handler{}, programgrpc.Server{}, programworker.Runner{}, err
		}
		mysqlPrograms := programmysql.NewProgramRepository(mysqlDB)
		programs = programes.NewProgramRepositoryWithIndex(mysqlPrograms, cfg.Elasticsearch.Addresses, cfg.Elasticsearch.Index)
		queryPrograms = programs
		indexer := programes.NewVersionedProgramIndexer(cfg.Elasticsearch.Addresses, cfg.Elasticsearch.Index, cfg.Elasticsearch.Version)
		syncer := programapp.NewProgramIndexSyncService(mysqlPrograms, indexer, cfg.Elasticsearch.BatchSize)
		indexRunner = programworker.NewIndexSyncRunner(syncer, cfg.Elasticsearch.SyncIntervalDuration(5*time.Minute))
		redisClient := thredis.NewClient(cfg.Redis)
		lua := cache.NewRedisLuaExecutor(redisClient)
		keys := cache.NewKeyBuilder(cfg.Redis.KeyPrefix)
		queryCache := programredis.NewProgramQueryCache(programs, redisClient, keys, 2*time.Minute)
		queryPrograms = queryCache
		inventoryStore := programredis.NewInventoryStore(redisClient, keys)
		bootstrapResult, err := programapp.NewInventoryBootstrapService(mysqlPrograms, inventoryStore, 500).Bootstrap(context.Background())
		if err != nil {
			return programhttp.Handler{}, programgrpc.Server{}, programworker.Runner{}, err
		}
		log.Printf("program inventory bootstrap completed: initialized=%d existing=%d", bootstrapResult.Initialized, bootstrapResult.Existing)
		inventory = programredis.NewInventoryLocker(
			keys,
			lua,
			15*60,
		)
		seatStates = programredis.NewSeatStateWriter(keys, lua)
		idempotency = programredis.NewIdempotencyStore(redisClient, keys)
		purchaseRateLimiter = programredis.NewPurchaseRateLimiter(redisClient, keys, 3, 6, 1000, 1500)
		inventoryReconciliation = programapp.NewInventoryReconciliationService(
			mysqlPrograms,
			inventoryStore,
			programmysql.NewReconciliationRecordRepository(mysqlDB),
			ids,
		)
		adminPrograms = programapp.NewProgramAdminService(mysqlPrograms, inventoryStore, ids)
		kafkaPublisher := mq.NewKafkaProducer(cfg.Kafka.Brokers, cfg.Kafka.ClientID)
		outboxRepository := programmysql.NewOutboxRepository(mysqlDB)
		publisher = programapp.NewOutboxPublisher(outboxRepository)
		outboxRunner = programworker.NewOutboxRunner(outboxRepository, kafkaPublisher)
		programEventConsumer := mq.NewKafkaConsumer(cfg.Kafka.Brokers, cfg.Kafka.GroupID)
		programEvents = programworker.NewProgramEventRunner(programEventConsumer, cfg.Kafka.Topics["program_changed"], indexer, queryCache)
		ticketUserClient, err := programrpc.NewTicketUserClient(cfg.GRPCUpstreams["user-service"])
		if err != nil {
			return programhttp.Handler{}, programgrpc.Server{}, programworker.Runner{}, err
		}
		ticketUsers = ticketUserClient
	} else {
		memoryPrograms := memory.NewProgramRepository()
		memoryInventory := memory.NewInventoryLocker()
		memoryInventory.Seed(1, 1000)
		memoryInventory.Seed(2, 2000)
		programs = memoryPrograms
		queryPrograms = memoryPrograms
		inventory = memoryInventory
		inventoryReconciliation = programapp.NewInventoryReconciliationService(
			memoryPrograms,
			memoryInventory,
			memory.NewReconciliationRecordRepository(),
			ids,
		)
		adminPrograms = programapp.NewProgramAdminService(memoryPrograms, memoryInventory, ids)
		publisher = mq.NewMemoryBroker()
		ticketUsers = memory.TicketUserSource{}
		idempotency = memory.NewIdempotencyStore()
	}
	usecase := programapp.NewCreateOrderUsecase(
		idgen.NewOrderNumberGenerator(ids),
		ids,
		inventory,
		publisher,
		programapp.NewProgramPricingService(programs),
	).WithValidator(programapp.NewPurchaseValidationPipeline(
		programapp.NewPurchaseStructureRule(programapp.DefaultMaxTicketsPerOrder),
		programapp.NewPurchaseCatalogRule(programs),
		programapp.NewTicketUserOwnershipRule(ticketUsers),
	)).WithIdempotency(idempotency).WithRateLimiter(purchaseRateLimiter)
	queries := programapp.NewProgramQueryService(queryPrograms)
	runner := programworker.NewRunner(indexRunner).WithOutbox(outboxRunner).WithProgramEvents(programEvents)
	return programhttp.NewHandler(usecase, queries).WithAdmin(adminPrograms), programgrpc.NewServer(queries, usecase, seatStates, inventoryReconciliation).WithReservations(inventory), runner, nil
}
