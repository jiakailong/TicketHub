# TicketHub 学习 Kubernetes 分步计划

> 目标：系统学习 Kubernetes，最终把本项目由 `docker-compose.yml` 组织的中间件全部迁到 k8s 管理。
> 环境：Windows + WSL2 + Docker Desktop，本地集群使用 **minikube（docker 驱动）**。
> 学习原则：**概念 → kubectl 实操 → 手写 manifest → 迁移本项目中间件**。不要一上来啃控制器源码。

---

## 一、项目现状盘点

| 部分 | 现状 | 位置 |
| --- | --- | --- |
| 中间件（etcd / APISIX / MySQL / Redis / Kafka / Elasticsearch / Prometheus / Grafana，共 8 个） | 全部由 Docker Compose 管理，named volume 存数据 | `docker-compose.yml` |
| 应用（9 个 Go 微服务） | 已有 k8s 雏形，但只覆盖 4 个服务（user/program/order/pay） | `deploy/k8s/tickethub-services.yaml`、`deploy/k8s/namespace.yaml` |
| Helm chart | 已有骨架：namespace + deployment + service 三个 template，values 列了 9 个服务 | `deploy/helm/tickethub/` |
| 应用镜像构建 | 多阶段 Dockerfile，`make image SERVICE=xxx` 构建 `tickethub/<svc>:local` | `deploy/docker/service.Dockerfile`、Makefile `image` target |
| 配置与密钥 | `.env`（已 gitignore），`scripts/generate-local-env.sh` 生成随机凭据 | `.env.example` |

**结论**：应用侧已有半成品，本次学习的主线是 **把 8 个中间件逐个迁进 k8s**，并顺手补全应用侧的 chart。

### 迁移时要注意的项目细节

- **APISIX 路由**：通过 `scripts/apisix-bootstrap.sh` 用 Admin API 写入（`deploy/docker/apisix/routes.yaml`），不是挂载进容器的——迁移后沿用此脚本，只改 Admin endpoint。
- **APISIX 配置**：`deploy/docker/apisix/config.yaml` 中的 etcd 地址、`deploy/docker/apisix/routes.yaml` 中的 `redis_host` / upstream 地址，迁移后都要从 compose 服务名/`host.docker.internal` 改成集群内 Service DNS。
- **Prometheus 抓取目标**：目前 `host.docker.internal:8001~8008`（应用跑在宿主机时）与 `tickethub-apisix:9091`。应用全部入集群后，改成各 Service 的 DNS。
- **MySQL 初始化**：`deploy/docker/mysql/init/` 下 7 个 SQL 由官方镜像 `docker-entrypoint-initdb.d` 机制执行，k8s 里用 ConfigMap 挂载到同一路径即可，行为一致。
- **Kafka 是 KRaft 模式**（单节点，broker+controller 合一），k8s 中需 StatefulSet + headless Service 固定身份。
- **Secret 清单**：`.env.example` 中的 `TICKETHUB_MYSQL_*`、`TICKETHUB_JWT_SECRET`、`TICKETHUB_PRIVACY_*`、`TICKETHUB_REGISTER_PROTECTION_HMAC_SECRET`、`APISIX_ADMIN_KEY`、`TICKETHUB_GRAFANA_ADMIN_*` 都应落入 k8s Secret。

---

## 二、总体路线图

| 阶段 | 内容 | 预估 | 产出 |
| --- | --- | --- | --- |
| 0 | 搭本地集群（minikube） | 半天 | 能 `kubectl get nodes`，会用 dashboard |
| 1 | 核心对象，用现有 manifest 当教材 | 1~2 周 | 亲手把 `deploy/k8s/` 的 4 个服务跑起来 |
| 2 | 中间件逐个迁移（本项目主线） | 2~4 周 | 8 个中间件全部以 k8s manifest 运行 |
| 3 | Helm 化 + 进阶 | 持续 | 统一 chart，HPA、Ingress、GitOps |

每课统一工作流：`写 manifest → kubectl apply --dry-run=client -f x.yaml 校验 → apply → kubectl get -w 观察 → 出错用 describe/logs 排查 → 验收 → 清理`。

---

## 阶段 0：搭建本地集群

### 0.1 安装 minikube

Windows（管理员 PowerShell）任选其一：

```powershell
choco install minikube        # 或
winget install minikube
```

kubectl 已随 Docker Desktop 提供（`C:\Program Files\Docker\Docker\resources\bin\kubectl.exe`），建议再装独立版本避免版本过旧：`choco install kubernetes-cli`。

helm 暂不装，阶段 3 前再装：`choco install kubernetes-helm`。

### 0.2 启动集群（docker 驱动）

> 本机网络对 Docker Hub / 阿里云 OSS 直连受限，以下参数是实测可用组合：
> kicbase 基础镜像从阿里云拉取并 tag 成本地 `gcr.io/k8s-minikube/kicbase:v0.0.50`，
> k8s 组件镜像走阿里云仓库，二进制走 dl.k8s.io。已固化为 `make k8s-start`。

```bash
# 首次（或需要重装基础镜像时）：
docker pull registry.cn-hangzhou.aliyuncs.com/google_containers/kicbase:v0.0.50
docker tag registry.cn-hangzhou.aliyuncs.com/google_containers/kicbase:v0.0.50 gcr.io/k8s-minikube/kicbase:v0.0.50

# 启动（参数已固化在 Makefile）：
make k8s-start
# 等价于：
minikube start --driver=docker --cpus=4 --memory=5120 \
  --kubernetes-version=v1.31.3 \
  --image-repository=registry.cn-hangzhou.aliyuncs.com/google_containers \
  --binary-mirror=https://dl.k8s.io \
  --preload=false
```

> 注意：minikube 若检测到镜像仓库是阿里云，会自动把 kubelet/kubectl 二进制源推断为阿里云 OSS（其版本滞后，会 404），
> 必须用 `--binary-mirror=https://dl.k8s.io` 显式覆盖。

### 0.3 验证与工具

```powershell
minikube status
kubectl get nodes
kubectl get pods -A                    # 看 kube-system 里的系统组件
minikube dashboard                     # 可视化面板（学习期很直观）
minikube ssh "sudo sysctl -w vm.max_map_count=262144"   # 给 Elasticsearch 用，阶段 2 会用到
```

### 0.4 验收

- [ ] `kubectl get nodes` 显示 READY 的节点
- [ ] 能说出集群里至少 5 个系统组件 pod 的名字和作用
- [ ] 打开过 dashboard，看到 node/pod 概览

---

## 阶段 1：核心对象（用现有 manifest 当教材）

> 项目文件 `deploy/k8s/tickethub-services.yaml` 是现成教材，每个概念都能在里头找到对应物。

### 1.1 kubectl 基础 + Namespace

- 概念：namespace 是资源隔离边界；kubectl 增删查改的对象操作。
- 项目对应：`deploy/k8s/namespace.yaml`（tickethub）。
- 练习：
  ```powershell
  kubectl apply -f deploy/k8s/namespace.yaml
  kubectl get ns
  kubectl create deployment nginx-test --image=nginx -n tickethub   # 练习用
  kubectl get pods -n tickethub -w
  kubectl delete deployment nginx-test -n tickethub
  ```
- 作业：给 `kubectl` 配置 namespace 默认值（`kubectl config set-context --current --namespace=tickethub`），理解为什么这样方便。

### 1.2 Pod

- 概念：k8s 最小的调度单位；一个 Pod 可含多容器（共享网络/IP）；Pod 是"临时"的，不直接管理。
- 练习：`kubectl run busybox --image=busybox -it --rm -- sh`，在 Pod 里 `nslookup kubernetes.default` 看集群 DNS。
- 作业：解释"为什么不直接创建 Pod 跑服务"。

### 1.3 Deployment / ReplicaSet

- 概念：Deployment 声明期望状态（副本数、镜像版本），ReplicaSet 保证副本数，滚动更新/回滚由 Deployment 负责。
- 项目对应：`deploy/k8s/tickethub-services.yaml` 中每个 Deployment（user-service 2 副本、program-service 3 副本…）。
- 练习（把现有 4 个服务真正跑起来）：
  ```powershell
  # 先构建镜像（Makefile 已有 target），minikube 用 docker 驱动，镜像就在同一个 daemon 里
  make image SERVICE=user-service
  kubectl apply -f deploy/k8s/tickethub-services.yaml
  kubectl get deploy,rs,pods -n tickethub
  kubectl rollout status deployment/user-service -n tickethub
  ```
- 作业：`kubectl scale deployment/user-service --replicas=1` 再观察 RS/Pod 变化；`kubectl rollout history` 理解版本管理。

### 1.4 Service

- 概念：Service 是稳定访问入口，把 label 选中的 Pod 聚合成一个 DNS 名；ClusterIP / NodePort / LoadBalancer 三种类型。
- 项目对应：同一个 yaml 里每个 Service（`app: user-service` 选中同名 Deployment 的 Pod）。
- 练习：
  ```powershell
  kubectl get svc -n tickethub
  kubectl port-forward svc/user-service 18001:8001 -n tickethub   # 本地访问集群内服务
  kubectl exec -it deploy/user-service -n tickethub -- curl http://user-service:8001/healthz
  ```
- 作业：讲清楚 Service 的 `selector` 怎么和 Deployment 的 `matchLabels` 对上的。

### 1.5 探针（readiness / liveness）

- 概念：readinessProbe 决定是否把 Pod 纳入 Service 流量；livenessProbe 决定是否重启；项目里 `httpGet /readyz`、`/healthz`。
- 练习：`kubectl get pods -n tickethub` 观察刚启动时的 `0/1` → `1/1`；`kubectl describe pod` 看探针判定过程。
- 作业：为什么 "readiness 失败 ≠ 重启，liveness 失败 = 重启"？各自适合什么场景？

### 1.6 ConfigMap / Secret

- 概念：配置与密钥从镜像中解耦；ConfigMap 明文、Secret base64 编码（不是加密）；挂载方式 `env` / `volumeMounts`。
- 项目对应：`.env.example` 中所有 `TICKETHUB_*` 变量；compose 里挂载的 `./deploy/docker/xxx/*.conf` 配置文件。
- 练习：
  ```powershell
  kubectl create secret generic tickethub-secrets --from-env-file=.env -n tickethub --dry-run=client -o yaml
  kubectl get secret tickethub-secrets -n tickethub -o yaml   # 观察 base64
  ```
- 作业：列一张表，把 `.env.example` 的每个变量归类为"ConfigMap 还是 Secret"。

### 1.7 调试三板斧

- `kubectl logs` / `kubectl exec -it` / `kubectl describe`，加上 `kubectl get events --sort-by=.lastTimestamp`。
- 练习：故意把某个 Deployment 的镜像名写错，走一遍"describe 看 ImagePullBackOff → 改对 → rollout"的排错流程。

### 1.8 滚动更新与回滚

- 概念：`strategy: RollingUpdate`（maxUnavailable / maxSurge）、版本回滚。
- 练习：改镜像 tag 触发更新，`kubectl rollout status` 观察；`kubectl rollout undo deployment/user-service` 回滚。

### 阶段 1 验收

- [ ] 4 个服务（user/program/order/pay）在 tickethub namespace 里 Running，`port-forward` 后能访问
- [ ] 不查资料能默写一个 "Deployment + Service + 探针" 的最小 manifest
- [ ] 能解释 ConfigMap 和 Secret 的区别、Service 三种类型

---

## 阶段 2：中间件迁移（本项目主线）

### 2.0 迁移方法论：compose → k8s 概念映射表

| Docker Compose | Kubernetes | 本项目的例子 |
| --- | --- | --- |
| `service` | Deployment + Service | `tickethub-redis` |
| `image: tag` | `containers[].image` + `imagePullPolicy` | `redis:7.2` |
| `ports: "127.0.0.1:9080:9080"` | Service（port/targetPort）+ port-forward / NodePort / Ingress | APISIX 9080 |
| `environment:` | `env` / `envFrom`（ConfigMap/Secret） | MySQL 密码、APISIX admin key |
| `volumes: ./x:/y`（配置） | ConfigMap / Secret 挂载 | `redis.conf`、`prometheus.yml`、`config.yaml` |
| `volumes: named:/data`（数据） | PersistentVolumeClaim | `tickethub_mysql_data` 等 7 个 |
| `networks: tickethub-net` | 默认扁平网络 + Service DNS | 服务名 `tickethub-mysql` 可直接映射成 Service 名 |
| `depends_on: condition: service_healthy` | readinessProbe / initContainers | APISIX 等 etcd 健康 |
| `healthcheck:` | readinessProbe + livenessProbe | mysqladmin ping、redis-cli ping |
| `extra_hosts: host.docker.internal` | Service DNS（集群内不需要） | APISIX 的 upstream、Prometheus targets |
| `restart: always` | 不需要（Deployment 自愈）+ livenessProbe | 全部 |
| `docker compose scale` | `replicas` + HPA | 阶段 3 |

**通用迁移步骤**（每个中间件都走一遍）：
1. 读 compose 里该服务的配置：image、env、volume、healthcheck、网络依赖
2. 写 k8s manifest（优先手写，再和 `kubectl create ... --dry-run=client -o yaml` 对照学习）
3. `kubectl apply --dry-run=client` 校验 → apply
4. 用等价命令验证：`redis-cli ping`、`mysqladmin ping`、`curl localhost:9200`、`kafka-topics.sh --list`…
5. 验证通过后，把 compose 里对应服务删除，其余服务改连集群内地址

### 2.1 Redis（无状态，最简单，第一个练手）

- compose 现状：`redis:7.2` + `redis.conf` 挂载 + `/data` volume + `redis-cli ping` healthcheck。
- k8s 设计：Deployment(1) + Service + ConfigMap(`redis.conf`) + PVC(`/data`)。
- 练习：
  ```powershell
  kubectl create configmap redis-config --from-file=deploy/docker/redis/redis.conf -n tickethub --dry-run=client -o yaml > deploy/k8s/redis.yaml
  # 手写 Deployment/Service/PVC 追加到 redis.yaml 再 apply
  kubectl exec -it deploy/tickethub-redis -n tickethub -- redis-cli ping   # 期望 PONG
  ```
- 作业：解释"为什么 Redis 这种也能用 Deployment 而 MySQL 不行"。

### 2.2 MySQL（第一个 StatefulSet）

- compose 现状：`mysql:8.4` + 密码 env + `init/` 7 个 SQL + `/var/lib/mysql` volume。
- k8s 设计：StatefulSet(1) + Service + Secret(密码) + ConfigMap(7 个 init SQL) + PVC。
- 练习：
  ```powershell
  kubectl create configmap mysql-init --from-file=deploy/docker/mysql/init -n tickethub --dry-run=client -o yaml
  # volumeMounts 挂到 /docker-entrypoint-initdb.d（和 compose 行为一致）
  kubectl exec -it tickethub-mysql-0 -n tickethub -- mysqladmin ping -h 127.0.0.1 -uroot -p$MYSQL_ROOT_PASSWORD
  # 验证 init SQL 生效：查表数量 / 某张种子表
  ```
- 作业：说清 StatefulSet 和 Deployment 的区别（稳定网络标识 `tickethub-mysql-0`、稳定 PVC、有序部署）。

### 2.3 Prometheus + Grafana（ConfigMap 挂载典型）

- compose 现状：`prometheus.yml` 挂载、Grafana provisioning + dashboards 挂载。
- k8s 设计：两个 Deployment + 两个 Service + 多个 ConfigMap + Grafana PVC。
- 练习：`prometheus.yml` 的 targets 先保留原样（应用未迁移时 `host.docker.internal` 在 docker 驱动下仍可通），等应用入集群后再统一改成 Service DNS。
- 作业：理解 `tickethub-apisix:9091` 这个 target 迁移后要变成什么地址。

### 2.4 etcd（APISIX 的依赖）

- compose 现状：单节点 etcd + healthcheck；APISIX 通过 etcd 存配置。
- k8s 设计：StatefulSet(1) + Service（**Service 名必须叫 `tickethub-etcd`**）；APISIX 的 `config.yaml` 里 etcd 地址本来就是 `http://tickethub-etcd:2379`，**Service 名保持一致就不用改配置**——这是迁移的核心技巧。
- 作业：验证 `etcdctl endpoint health`；说明"为什么 APISIX 的 ConfigMap 改动要重启/热加载"。

### 2.5 Elasticsearch

- compose 现状：`elasticsearch:8.14.3` + `discovery.type=single-node` + `xpack.security.enabled=false` + `ES_JAVA_OPTS` 限内存。
- k8s 设计：StatefulSet(1) + Service + PVC + 同样的 env。
- 注意：先执行阶段 0 的 `minikube ssh ... vm.max_map_count=262144`；内存要给足（`-Xms512m -Xmx512m` 已有）。
- 作业：`curl http://elasticsearch:9200` 返回集群名；复跑 `scripts/elasticsearch-create-program-index.sh` 验证索引创建。

### 2.6 Kafka（KRaft，最难，放最后）

- compose 现状：单节点 KRaft（`KAFKA_PROCESS_ROLES: broker,controller`），`EXTERNAL://localhost:9094` 供宿主机访问。
- k8s 设计：StatefulSet(1) + headless Service；`KAFKA_ADVERTISED_LISTENERS` 需改成 Pod DNS（如 `PLAINTEXT://tickethub-kafka-0.tickethub-kafka.tickethub.svc.cluster.local:9092`）；内部 listener 供集群内应用用，外部访问用 `kubectl port-forward` 或额外 NodePort。
- 作业：复跑 `scripts/kafka-create-topics.sh` 建 topic，用 `kafka-topics.sh --list` 验证。

### 2.7 APISIX（对外入口，引入 Ingress 概念）

- compose 现状：`apisix:3.9.0` + `config.yaml` 挂载 + Admin API(9180) + 对外 9080/9443。
- k8s 设计：Deployment + Service + ConfigMap(`config.yaml`)；etcd 地址天然是 `tickethub-etcd:2379`，Service 命名保持一致即可，配置文件原样迁移。
- 对外暴露（三种方式都要会）：
  ```powershell
  kubectl port-forward svc/tickethub-apisix 9080:9080 -n tickethub     # 调试用
  minikube service tickethub-apisix -n tickethub                       # NodePort 方式
  # Ingress：学完阶段 3.2 后用 apisix-ingress-controller 或 nginx-ingress 接入
  ```
- 路由初始化：改 `scripts/apisix-bootstrap.sh` 的 Admin endpoint（`http://tickethub-apisix:9180`），把 `routes.yaml` 里的 `host.docker.internal:8080` 换成 `gateway-bff:8080`、`redis_host` 换成 `tickethub-redis`。

### 2.8 应用入集群 + 全链路验证（收尾）

- 目标：`TICKETHUB_ADAPTER_MODE=infra` 下完整链路在集群内跑通：APISIX → gateway-bff → user/order/pay → MySQL/Redis/Kafka/ES。
- 步骤：把现有 `deploy/k8s/tickethub-services.yaml` 扩成 9 个服务（或直接用 helm chart 渲染，见阶段 3）；把 ConfigMap/Secret 挂进应用；Prometheus targets 全改成集群内 DNS。
- 验收：APISIX 路由限流正常（`limit-req` 依赖 Redis）、下单链路走通、Grafana 能看到服务指标。

### 阶段 2 验收

- [x] 8 个中间件全部以 k8s 对象运行，compose 文件里只剩可以删的服务
- [x] 每个中间件的健康检查命令在集群内验证通过
- [x] 能向别人讲清：为什么 MySQL/etcd/ES/Kafka 用 StatefulSet，Redis 用 Deployment 就行

### 实际迁移记录（2026-08 完成，全部在 `deploy/k8s/`）

| 中间件 | manifest | 核心对象 | 实测踩坑 |
| --- | --- | --- | --- |
| Redis | `redis.yaml` | Deployment+Service+ConfigMap+PVC | 无 |
| MySQL | `mysql.yaml` | StatefulSet+headless+volumeClaimTemplates | ① compose 的 `command` 在 k8s 用 `args`（追加 ENTRYPOINT）；② StatefulSet 更新后 Pod 不自动重建 → `kubectl delete pod` 强制 |
| Prometheus | `observability.yaml` | Deployment+ConfigMap(subPath) | 镜像拉取偶发 EOF → 节点内手动重试 |
| Grafana | `observability.yaml` | Deployment+多 ConfigMap+Secret+PVC | 同上 |
| etcd | `etcd.yaml` | StatefulSet+普通 Service | 镜像**无 ENTRYPOINT** → 必须 `command` 完整写（`docker inspect` 先查） |
| Elasticsearch | `elasticsearch.yaml` | StatefulSet | 镜像源 `docker.elastic.co` 下载慢 → 多轮重试；vm.max_map_count minikube 已满足 |
| Kafka | `kafka.yaml` | StatefulSet+headless+**publishNotReadyAddresses** | ① headless DNS 只解析 Ready Pod → KRaft 自举死锁 → `publishNotReadyAddresses: true`；② exec 探针默认 timeout 1s 不够 JVM → `timeoutSeconds: 15` |
| APISIX | `apisix.yaml` | Deployment+ConfigMap+Secret | 路由初始化脚本默认指向 9181（compose）→ 用 `APISIX_ADMIN=http://127.0.0.1:9180` 覆盖写 k8s；`scripts/apisix-init-k8s.sh` |

**经验沉淀**：
- Service 名 = compose 服务名 → 应用/配置文件里的地址（如 `tickethub-redis:6379`、`tickethub-etcd:2379`）**零改动**直接用
- Prometheus 的 `tickethub-apisix:9091` target 因同名 Service 自动恢复抓取；`host.docker.internal:*` 待应用入集群后改为 Service DNS
- wsl.exe 传参给 bash 会吃掉 `$变量` → 涉及变量脚本一律写成文件再执行

---

## 阶段 3：Helm 化与进阶

### 3.1 装 helm 并补全 `deploy/helm/tickethub`

- 现状：`deployment.yaml` + `service.yaml` 已用 `range` 渲染全部 9 个服务，缺 ConfigMap/Secret/Ingress/HPA。
- 练习：给 chart 加 `templates/secret.yaml`（从 `.env.example` 生成占位）、`templates/configmap.yaml`、`templates/hpa.yaml`；`helm template tickethub deploy/helm/tickethub` 渲染检查。
- 作业：解释 Helm 的 values/templates/rendered manifest 三层关系。

### 3.2 Ingress

- 练习：`minikube addons enable ingress`，为 gateway-bff 写 Ingress 规则（`/api/*` → gateway-bff），替换 `port-forward`。
- 进阶（选做）：研究 APISIX 作为 Ingress Controller 的方案（apisix-ingress-controller），与本项目 APISIX 网关定位契合。

### 3.3 资源管理与自动扩缩

- requests/limits：给所有容器补资源配额，观察 `kubectl describe node` 的分配情况。
- HPA：`minikube addons enable metrics-server`，基于 CPU 给 program-service 配 HPA；进阶可接 Prometheus 自定义指标。

### 3.4 可观测性与生产化（选做）

- 用 `kube-prometheus-stack`（helm chart）替换裸 Prometheus/Grafana，获得 kube-state-metrics + 告警。
- 滚动更新策略、PodDisruptionBudget、PodAntiAffinity。
- GitOps：ArgoCD 管理集群内 manifest（选学）。

---

## 三、概念速查表（随时回查）

| 想做的事 | compose 写法 | k8s 写法 |
| --- | --- | --- |
| 跑一个副本 | `service` | Deployment `replicas: 1` |
| 服务发现 | compose 网络内用服务名 | Service + DNS `name.namespace.svc.cluster.local` |
| 对外暴露端口 | `ports:` | Service NodePort / LoadBalancer / Ingress / port-forward |
| 存配置 | `volumes: ./x:/etc/x:ro` | ConfigMap 挂载 |
| 存密钥 | `.env` | Secret |
| 持久化 | named volume | PVC + StatefulSet（有状态）/ Deployment（共享 PVC） |
| 健康检查 | `healthcheck:` | readinessProbe / livenessProbe |
| 启动顺序 | `depends_on` | readinessProbe 编排 / initContainers |
| 自愈 | `restart: always` | Deployment 控制器 + livenessProbe |
| 扩缩容 | `docker compose scale` | `kubectl scale` / HPA |
| 滚动发布 | 手动重建 | `kubectl rollout`（Deployment 内置） |

---

## 四、推荐资料

- **《Kubernetes in Action》第 2 版**（Marko Lukša）：系统学习首选书籍。
- **极客时间《深入剖析 Kubernetes》**（张磊）：中文系统课，重原理。
- **官方文档**：[kubectl cheat sheet](https://kubernetes.io/docs/reference/kubectl/cheat-sheet/)、minikube 文档。
- 社区中文资源：Kubernetes 官方中文文档、`kubectl explain <resource>` 是随身手册。

## 五、学习节奏建议

- 阶段 0~1 每天 1~2 小时，概念配实操；**阶段 1 的 1.8 之前不要进阶段 2**。
- 阶段 2 每 2~3 天迁一个中间件，顺序：Redis → MySQL → Prometheus/Grafana → etcd → ES → Kafka → APISIX。
- 卡住时用调试三板斧（logs/exec/describe），能自己排错才算学会。
- 每个中间件迁移完成，顺手更新本仓库：把 manifest 落到 `deploy/k8s/`（或 chart），并在 Makefile 加 `k8s-*` target 固化工作流。
