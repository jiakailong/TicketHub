SHELL := /bin/bash
PROJECT := tickethub

ifneq (,$(wildcard .env))
include .env
export
endif

.PHONY: test test-integration fmt vet build image proto docker-config docker-up docker-down bootstrap-infra web-install web-dev web-build web-test privacy-expand privacy-migrate privacy-contract k8s-start k8s-stop k8s-delete

test:
	go test ./...

test-integration:
	TICKETHUB_INTEGRATION=1 go test -count=1 -timeout=2m -tags=integration -v ./app/user-service/internal/infrastructure/mysql
	TICKETHUB_INTEGRATION=1 go test -count=1 -timeout=2m -tags=integration -v ./tests/integration

fmt:
	go fmt ./...

vet:
	go vet ./...

build:
	@set -e; \
	for svc in gateway-bff user-service program-service order-service pay-service base-data-service customize-service admin-service migrate-service; do \
		echo "building $$svc"; \
		go build -o bin/$$svc ./app/$$svc/cmd/server; \
	done

image:
	@if [ -z "$(SERVICE)" ]; then echo "SERVICE is required, for example: make image SERVICE=user-service"; exit 1; fi
	docker build -f deploy/docker/service.Dockerfile --build-arg SERVICE=$(SERVICE) -t tickethub/$(SERVICE):local .

proto:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.33.0
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.3.0
	cd api/proto && PATH="$$(go env GOPATH)/bin:$$PATH" go run github.com/bufbuild/buf/cmd/buf@v1.32.2 generate

docker-config:
	docker compose -p $(PROJECT) config

docker-up:
	scripts/docker-compose-up.sh

docker-down:
	docker compose -p $(PROJECT) down

# 本地 k8s 学习环境：minikube（docker 驱动，k8s v1.31.3）
# 说明：组件镜像走阿里云加速，二进制走 dl.k8s.io（本机网络 Docker Hub/阿里云 OSS 受限时可用）
K8S_VERSION := v1.31.3
K8S_MEMORY := 5120

k8s-start:
	minikube start --driver=docker --cpus=4 --memory=$(K8S_MEMORY) \
		--kubernetes-version=$(K8S_VERSION) \
		--image-repository=registry.cn-hangzhou.aliyuncs.com/google_containers \
		--binary-mirror=https://dl.k8s.io \
		--preload=false

k8s-stop:
	minikube stop

k8s-delete:
	minikube delete

bootstrap-infra:
	scripts/bootstrap-local-dependencies.sh

web-install:
	pnpm --dir web install

web-dev:
	pnpm --dir web dev

web-build:
	pnpm --dir web build

web-test:
	pnpm --dir web test:e2e

privacy-expand:
	docker exec -i tickethub-mysql sh -c 'exec mysql -uroot -p"$$MYSQL_ROOT_PASSWORD"' < deploy/docker/mysql/init/006_user_privacy_expand.sql

privacy-migrate:
	TICKETHUB_ADAPTER_MODE=infra go run ./app/user-service/cmd/privacy-migrate

privacy-contract:
	docker exec -i tickethub-mysql sh -c 'exec mysql -uroot -p"$$MYSQL_ROOT_PASSWORD"' < deploy/docker/mysql/init/007_user_privacy_contract.sql
