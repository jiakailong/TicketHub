package main

import (
	"context"
	"flag"
	"log"
	"net/http"

	khttp "github.com/go-kratos/kratos/v2/transport/http"

	kgrpc "github.com/go-kratos/kratos/v2/transport/grpc"
	userv1 "tickethub/api/proto/user/v1"
	userapp "tickethub/app/user-service/internal/application"
	"tickethub/app/user-service/internal/infrastructure/memory"
	usermysql "tickethub/app/user-service/internal/infrastructure/mysql"
	userredis "tickethub/app/user-service/internal/infrastructure/redis"
	"tickethub/app/user-service/internal/infrastructure/security"
	usergrpc "tickethub/app/user-service/internal/interfaces/grpc"
	userhttp "tickethub/app/user-service/internal/interfaces/http"
	"tickethub/pkg/auth"
	"tickethub/pkg/bootstrap"
	"tickethub/pkg/config"
	"tickethub/pkg/db"
	"tickethub/pkg/httpx"
	"tickethub/pkg/idgen"
	"tickethub/pkg/privacy"
	thredis "tickethub/pkg/redis"
)

func main() {
	conf := flag.String("conf", "app/user-service/configs/config.yaml", "config file path")
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
		userv1.RegisterUserServiceServer(server, grpcServer)
	}); err != nil {
		log.Fatal(err)
	}
}

func registerHTTP(server *khttp.Server) {
	server.HandleFunc("/v1/user/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"service":"user-service","status":"ok"}`))
	})
}

func buildHandlers(cfg config.Config) (userhttp.Handler, usergrpc.Server, error) {
	clientIPs, err := httpx.NewClientIPResolver(cfg.Security.TrustedProxyCIDRs)
	if err != nil {
		return userhttp.Handler{}, usergrpc.Server{}, err
	}
	ids, err := idgen.NewSnowflake(1)
	if err != nil {
		return userhttp.Handler{}, usergrpc.Server{}, err
	}
	var users userapp.UserCommandService
	if cfg.UseInfrastructure() {
		client, err := db.OpenMySQL(context.Background(), cfg.MySQL)
		if err != nil {
			return userhttp.Handler{}, usergrpc.Server{}, err
		}
		protector, err := privacy.NewProtector(cfg.Privacy.ActiveKeyVersion, cfg.Privacy.EncryptionKeys, cfg.Privacy.LookupKey)
		if err != nil {
			return userhttp.Handler{}, usergrpc.Server{}, err
		}
		users = userapp.NewUserCommandService(
			ids,
			usermysql.NewUserRepository(client, protector),
			security.NewBcryptPasswordHasher(0),
		).WithTicketUsers(usermysql.NewTicketUserRepository(client, protector))
		if cfg.Registration.Enabled {
			redisClient := thredis.NewClient(cfg.Redis)
			guard, err := userredis.NewRegistrationGuard(redisClient, cfg.Redis.KeyPrefix, cfg.Registration, protector.Lookup)
			if err != nil {
				_ = redisClient.Close()
				return userhttp.Handler{}, usergrpc.Server{}, err
			}
			if err := guard.Bootstrap(context.Background(), usermysql.NewMobileLookupScanner(client)); err != nil {
				log.Printf("registration Bloom bootstrap unavailable; falling back to MySQL precheck: %v", err)
			}
			users = users.WithRegistrationGuard(guard)
		}
	} else {
		users = userapp.NewUserCommandService(
			ids,
			memory.NewUserRepository(),
			security.NewBcryptPasswordHasher(0),
		).WithTicketUsers(memory.NewTicketUserRepository())
	}
	users = users.WithAdminMobiles(cfg.Auth.AdminMobiles).WithTokenManager(auth.NewTokenManager(cfg.Auth.JWTSecret))
	return userhttp.NewHandler(users).WithClientIPResolver(clientIPs), usergrpc.NewServer(users), nil
}
