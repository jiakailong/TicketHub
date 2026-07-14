package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"

	kgrpc "github.com/go-kratos/kratos/v2/transport/grpc"
	khttp "github.com/go-kratos/kratos/v2/transport/http"

	migratev1 "tickethub/api/proto/migrate/v1"
	migrateapp "tickethub/app/migrate-service/internal/application"
	"tickethub/app/migrate-service/internal/infrastructure/memory"
	migratemysql "tickethub/app/migrate-service/internal/infrastructure/mysql"
	migrategrpc "tickethub/app/migrate-service/internal/interfaces/grpc"
	migratehttp "tickethub/app/migrate-service/internal/interfaces/http"
	migrateworker "tickethub/app/migrate-service/internal/interfaces/worker"
	"tickethub/pkg/bootstrap"
	"tickethub/pkg/config"
	"tickethub/pkg/db"
	"tickethub/pkg/idgen"
	"tickethub/pkg/lock"
	thredis "tickethub/pkg/redis"
)

func main() {
	conf := flag.String("conf", "app/migrate-service/configs/config.yaml", "config file path")
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
		migratev1.RegisterMigrateServiceServer(server, grpcServer)
	}); err != nil {
		log.Fatal(err)
	}
}

func registerHTTP(server *khttp.Server) {
	server.HandleFunc("/v1/migrate/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"service":"migrate-service","status":"ok"}`))
	})
}

func buildHandlers(cfg config.Config) (migratehttp.Handler, migrategrpc.Server, migrateworker.Runner, error) {
	ids, err := idgen.NewSnowflake(8)
	if err != nil {
		return migratehttp.Handler{}, migrategrpc.Server{}, migrateworker.Runner{}, err
	}
	var repo migrateapp.MigrationRepository
	var locker lock.Locker
	if cfg.UseInfrastructure() {
		mysqlDB, err := db.OpenMySQL(context.Background(), cfg.MySQL)
		if err != nil {
			return migratehttp.Handler{}, migrategrpc.Server{}, migrateworker.Runner{}, err
		}
		repo = migratemysql.NewMigrationRepository(mysqlDB)
		locker = lock.NewRedisLocker(thredis.NewClient(cfg.Redis))
	} else {
		repo = memory.NewMigrationRepository()
		locker = lock.NewMemoryLocker()
	}
	service := migrateapp.NewMigrationService(repo)
	if cfg.UseInfrastructure() {
		service = service.WithAllowedTargets(configuredMigrationTargets(cfg.Sharding))
	}
	worker := migrateapp.NewMigrationWorker(repo, locker)
	return migratehttp.NewHandler(service, ids), migrategrpc.NewServer(service, ids), migrateworker.NewRunner(worker), nil
}

func configuredMigrationTargets(cfg config.ShardingConfig) []string {
	targets := make([]string, 0, len(cfg.Databases)*cfg.TableCount)
	for database := range cfg.Databases {
		for tableIndex := 0; tableIndex < cfg.TableCount; tableIndex++ {
			targets = append(targets, fmt.Sprintf("%s.%s_%d", database, cfg.TablePrefix, tableIndex))
		}
	}
	return targets
}
