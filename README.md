# TicketHub

TicketHub 是一个使用 Go 和 DDD 构建的高并发演出票务系统，覆盖用户注册登录、节目检索、选座购票、异步下单、支付回调、订单关闭、对账补偿和分片迁移等核心流程。

## 项目亮点

- **高并发购票**：Redis Lua 原子校验并锁定库存，Kafka 异步创建订单，通过幂等消费和失败回滚防止重复下单与超卖。
- **可靠订单闭环**：延迟队列关闭超时订单；支付回调按状态机推进，已取消订单收到成功回调时自动退款。
- **分库分表与扩容**：订单号携带用户分片基因，应用层路由到物理库表；迁移服务支持双写、批量复制、切换和恢复执行。
- **搜索与缓存**：Elasticsearch 支持节目搜索和游标分页，并以 MySQL 为权威数据源；Redis 承担库存、限流和热点数据访问。
- **安全与可观测性**：APISIX 统一入口并限流，JWT 完成身份认证，bcrypt 哈希密码，AES-GCM 加密隐私数据；Prometheus、Grafana、OpenTelemetry 和 Zap 提供监控与追踪能力。
- **DDD 微服务**：按用户、节目、订单、支付等业务边界拆分 9 个服务，内部采用 gRPC + Protobuf，外部提供 REST API。

## 技术栈

| 类型 | 技术 |
| --- | --- |
| 后端 | Go、Kratos、DDD、gRPC、Protobuf |
| 网关与消息 | APISIX、Kafka |
| 数据与缓存 | MySQL、Redis、Elasticsearch |
| 前端 | Vue 3、Vite、Pinia、Element Plus |
| 可观测性 | Prometheus、Grafana、OpenTelemetry、Zap |
| 部署 | Docker Compose、Kubernetes、Helm |

## 服务划分

`gateway-bff`、`user-service`、`program-service`、`order-service`、`pay-service`、`base-data-service`、`customize-service`、`admin-service`、`migrate-service`。

## 快速开始

环境要求：Go 1.25+、Docker Compose、pnpm、OpenSSL。

```bash
# 生成仅保存在本机的随机开发凭据
scripts/generate-local-env.sh

# 启动并初始化 MySQL、Redis、Kafka、Elasticsearch、APISIX 等依赖
make docker-up
make bootstrap-infra

# 验证、测试和构建后端
make test
make vet
make build

# 启动前端
make web-install
make web-dev
```

前端默认运行在 `http://127.0.0.1:5173`，API 入口为 `http://127.0.0.1:9080`。服务默认使用内存适配器；需要连接中间件时设置 `TICKETHUB_ADAPTER_MODE=infra`。

`.env` 已被 Git 忽略，禁止提交生产凭据。生产环境应通过 Kubernetes Secret、Vault 或云密钥管理服务注入 JWT、数据库和隐私加密密钥。

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

