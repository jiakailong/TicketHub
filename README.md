# TicketHub

TicketHub 是一个使用 Go 和 DDD 构建的高并发演出票务系统，覆盖用户注册登录、节目检索、选座购票、异步下单、支付回调、订单关闭、对账补偿和分片迁移等核心流程。

## 项目亮点

- **高并发购票**：Redis Lua 原子校验并锁定库存，Kafka 异步创建订单，通过幂等消费和失败回滚防止重复下单与超卖。
- **可靠订单闭环**：延迟队列关闭超时订单；支付回调按状态机推进，已取消订单收到成功回调时自动退款。
- **分库分表与扩容**：订单号携带用户分片基因，应用层路由到物理库表；迁移服务支持双写、批量复制、切换和恢复执行。
- **搜索与三级缓存**：Elasticsearch 支持节目搜索和游标分页；节目详情与票档采用 Ristretto + Redis + MySQL 三级缓存，通过分段读写锁、双重检测与 Redis 分布式锁防止热点缓存击穿。
- **安全与可观测性**：注册接口通过 Redis Lua 动态启用图形验证码，并以 IP/手机号双维限流和 Redis Bitmap Bloom 保护 MySQL；APISIX、JWT、bcrypt、AES-GCM、Prometheus、Grafana、OpenTelemetry 和 Zap 共同提供安全与观测能力。
- **DDD 微服务**：按用户、节目、订单、支付等业务边界拆分 9 个服务，内部采用 gRPC + Protobuf，外部提供 REST API。

## 技术栈

| 类型 | 技术 |
| --- | --- |
| 后端 | Go、Kratos、DDD、gRPC、Protobuf |
| 网关与消息 | APISIX、Kafka |
| 数据与缓存 | MySQL、Redis、Ristretto、Elasticsearch |
| 前端 | Vue 3、Vite、Pinia、Element Plus |
| 可观测性 | Prometheus、Grafana、OpenTelemetry、Zap |
| 部署 | Docker Compose、Kubernetes、Helm |

## 服务划分

`gateway-bff`、`user-service`、`program-service`、`order-service`、`pay-service`、`base-data-service`、`customize-service`、`admin-service`、`migrate-service`。

## 快速开始

环境要求：Go 1.25+、Docker Engine（含 compose v2 与 buildx 插件）、pnpm、OpenSSL。

### 一次性准备

```bash
# 生成仅保存在本机的随机开发凭据（写入 .env，已被 Git 忽略）
scripts/generate-local-env.sh

# Elasticsearch 要求宿主机 mmap 上限不低于 262144，否则容器启动后立即退出
sudo sysctl -w vm.max_map_count=262144
echo 'vm.max_map_count=262144' | sudo tee /etc/sysctl.d/99-elasticsearch.conf
```

镜像构建阶段的 Go 模块代理默认为 `goproxy.cn`，可用 `docker build --build-arg GOPROXY=...` 覆盖。

### 启动

```bash
# 构建 9 个服务镜像
make services-build

# 启动整个栈：8 个中间件 + 9 个业务服务
make docker-up

# 初始化中间件：建 Kafka topic、灌 Redis 库存、建 ES 索引、下发 APISIX 路由
make bootstrap-infra

# 启动前端
make web-install
make web-dev
```

### 访问入口

| 入口 | 地址 |
| --- | --- |
| 前端 | `http://127.0.0.1:5173` |
| API 网关（APISIX） | `http://127.0.0.1:9080` |
| Prometheus | `http://127.0.0.1:19090` |
| Grafana | `http://127.0.0.1:3000` |
| Elasticsearch | `http://127.0.0.1:9200` |

请求链路为 前端 → Vite proxy（`web/vite.config.js`）→ APISIX → `gateway-bff` → 各业务服务。APISIX 路由存放在 etcd 中而非配置文件，修改 `scripts/apisix-bootstrap.sh` 后必须执行 `make apisix-routes` 才会生效。

Prometheus 的宿主端口可用 `.env` 中的 `TICKETHUB_PROMETHEUS_PORT` 调整，容器内始终监听 9090，因此 Grafana 数据源地址不受影响。

### 常用命令

| 命令 | 作用 |
| --- | --- |
| `make docker-up` / `make docker-down` | 启停整个栈 |
| `make services-build` | 构建 9 个服务镜像 |
| `make services-up` / `make services-down` | 只启停 9 个业务服务，不动中间件 |
| `make services-ps` | 查看全部容器状态 |
| `make services-logs [SVC=order-service]` | 跟踪日志，缺省为全部服务 |
| `make services-restart [SVC=order-service]` | 重启服务 |
| `make apisix-routes` | 重新下发网关路由 |
| `make debug SVC=gateway-bff` | 附加临时调试容器 |
| `make image SERVICE=user-service` | 单独构建某个服务镜像 |
| `make test` / `make vet` / `make build` | 宿主机上验证与编译 |
| `make k8s-start` / `make k8s-stop` | 启停 minikube |

服务镜像基于 distroless，内部没有 shell，`docker exec` 无法进入。`make debug SVC=<服务名>` 会启动一个共享目标容器网络与 PID 命名空间的 Alpine 容器，可在其中访问 `localhost:<端口>`、通过 `/proc/1/root` 读取容器文件系统、通过 `/proc/1/environ` 核对实际生效的环境变量。

### 配置注入

`app/*/configs/config.yaml` 会被打进镜像，其中的 `127.0.0.1` 默认值仅适用于宿主机直跑。容器与集群内通过环境变量覆盖，由 `pkg/config/config.go` 的 `ApplyDefaults()` 在 YAML 解析之后应用，因此环境变量优先级高于配置文件：

- 密钥类走 compose 的 `env_file: .env`，等价于 Kubernetes 的 `envFrom: secretRef`
- 地址类走 compose 的 `environment:`，等价于 Kubernetes 的 `env:`

变量清单见 `docker-compose.yml` 顶部的 `x-app-env` 锚点，与 `deploy/k8s/tickethub-services.yaml` 保持一致，两边可互相迁移。

注意 `TICKETHUB_MYSQL_HOST` 在 `.env` 中是 `127.0.0.1:3306`，而 compose 与 Kubernetes 清单里都必须写成服务名 `tickethub-mysql:3306`，不能透传 `.env` 的值。

### 宿主机直跑模式

也可以只把中间件放进容器，服务用本机二进制运行：

```bash
make services-down
make build
TICKETHUB_ADAPTER_MODE=infra ./bin/user-service -conf app/user-service/configs/config.yaml
```

此模式下 `config.yaml` 的 `127.0.0.1` 默认值直接可用，中间件端口均已映射到宿主机回环。需要连接中间件时必须设置 `TICKETHUB_ADAPTER_MODE=infra`，否则服务使用内存适配器，日志无报错但不会读写任何存储。此时 APISIX 路由需改回宿主机地址：`BFF_UPSTREAM=host.docker.internal:8080 make apisix-routes`。

`.env` 已被 Git 忽略，禁止提交生产凭据。生产环境应通过 Kubernetes Secret、Vault 或云密钥管理服务注入 JWT、数据库、隐私加密和注册防护 HMAC 密钥；示例中的 `172.16.0.0/12` 仅用于 Docker Desktop，生产必须收窄为实际 APISIX/Ingress 网段。

## 项目结构

```text
TicketHub/
├── api/             # OpenAPI、Protobuf 契约
├── app/             # 九个微服务及其 DDD 分层实现
├── pkg/             # 鉴权、缓存、分片、锁、消息等共享组件
├── deploy/          # Docker、Kubernetes、Helm 配置
├── scripts/         # 初始化、冒烟测试和压测脚本
├── tests/           # 跨服务集成测试
└── web/             # Vue 3 用户端
```
