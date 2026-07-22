package main

import (
	"flag"
	"log"
	"net/http"

	khttp "github.com/go-kratos/kratos/v2/transport/http"

	gatewayhttp "tickethub/app/gateway-bff/internal/interfaces/http"
	"tickethub/pkg/auth"
	"tickethub/pkg/bootstrap"
	"tickethub/pkg/config"
	"tickethub/pkg/httpx"
)

func main() {
	conf := flag.String("conf", "app/gateway-bff/configs/config.yaml", "config file path")
	flag.Parse()

	cfg, err := config.Load(*conf)
	if err != nil {
		log.Fatal(err)
	}
	handler, err := gatewayhttp.NewHandlerWithGRPC(cfg.Upstreams, cfg.GRPCUpstreams, auth.NewTokenManager(cfg.Auth.JWTSecret))
	if err != nil {
		log.Fatal(err)
	}
	clientIPs, err := httpx.NewClientIPResolver(cfg.Security.TrustedProxyCIDRs)
	if err != nil {
		log.Fatal(err)
	}
	handler = handler.WithClientIPResolver(clientIPs)
	if err := bootstrap.RunHTTP(cfg, func(server *khttp.Server) {
		registerHTTP(server)
		handler.Register(server)
	}); err != nil {
		log.Fatal(err)
	}
}

func registerHTTP(server *khttp.Server) {
	server.HandleFunc("/v1/gateway/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"service":"gateway-bff","status":"ok"}`))
	})
}
