# TicketHub Kubernetes 学习复盘笔记

> 记录 2026-08 从"中间件用 Docker Compose 管理"到"8 个中间件全部由 Kubernetes 管理"的完整过程。
> 包含：环境搭建流程、服务迁移流程、常用命令、以及过程中踩过的每一个坑。
> 与 `k8s-learning-plan.md`（学习计划）配套：前者是"怎么做"，本文是"实际做了什么、遇到了什么"。

---

## 一、环境搭建流程（阶段 0）

### 1.1 最终环境

| 组件 | 版本/参数 | 位置 |
| --- | --- | --- |
| minikube | v1.38.1（docker 驱动） | WSL2 (Ubuntu-22.04)，用户 `jkl` |
| Kubernetes | **v1.31.3**（不是默认的 v1.35.1） | 单节点 minikube |
| kubectl | v1.36.3 | WSL2 `/usr/local/bin/kubectl` |
| Docker Desktop | 28.1.1（WSL2 集成已开启） | Windows |

### 1.2 启动命令（已固化到 Makefile，`make k8s-start`）

```bash
# 首次准备基础镜像（kicbase，Docker Hub 不通所以从阿里云拉）
docker pull registry.cn-hangzhou.aliyuncs.com/google_containers/kicbase:v0.0.50
docker tag registry.cn-hangzhou.aliyuncs.com/google_containers/kicbase:v0.0.50 gcr.io/k8s-minikube/kicbase:v0.0.50

# 启动集群
minikube start --driver=docker --cpus=4 --memory=5120 \
  --kubernetes-version=v1.31.3 \
  --image-repository=registry.cn-hangzhou.aliyuncs.com/google_containers \
  --binary-mirror=https://dl.k8s.io \
  --preload=false
```

### 1.3 搭建过程中遇到的坑（按时间顺序）

| # | 现象 | 根因 | 解决 |
| --- | --- | --- | --- |
| 1 | Docker Desktop 引擎起不来（`com.docker.service` Stopped） | Docker Desktop 首次运行需要管理员权限安装 Windows 服务 | 右键"以管理员身份运行" + 接受许可 |
| 2 | WSL2 里 `docker` 命令找不到 | Docker Desktop 的 WSL 集成未开启 | Settings → Resources → WSL Integration 勾选 Ubuntu-22.04 |
| 3 | WSL2 里 docker 报 `permission denied` | 用户 `jkl` 不在 docker 组 | `sudo usermod -aG docker jkl` |
| 4 | `minikube start` 报 `RSRC_OVER_ALLOC_MEM` | WSL2 默认内存上限 ~8GB，请求了 8192MB | 降到 `--memory=5120`（16GB 宿主机可配 `.wslconfig` 提到 12GB） |
| 5 | `minikube start` 报 `DRV_AS_ROOT` | WSL 默认用户是 root，docker 驱动拒绝 root | 全程用 `wsl -u jkl` 操作 minikube |
| 6 | 拉 kicbase 卡死（25KB/s） | Docker Hub 直连不通 | **阿里云拉 kicbase + tag 成本地镜像**，minikube 检测到本地镜像直接用 |
| 7 | kubelet 二进制下载 404（阿里云 OSS） | `--image-mirror-country=cn` 把二进制源也切到阿里云 OSS，但 OSS 没有 v1.35.1 | 换 `--kubernetes-version=v1.31.3`（OSS 有） |
| 8 | 还是从阿里云 OSS 下载二进制 404 | minikube 看到镜像仓库是阿里云就**自动推断**二进制源为阿里云 OSS | 显式 `--binary-mirror=https://dl.k8s.io` 覆盖 |
| 9 | `kubectl get ns` 报 `localhost:8080 connection refused` | root 用户没有 kubeconfig（minikube 是 jkl 启动的，配置在 `/home/jkl/.kube/config`） | `cp /home/jkl/.kube/config /root/.kube/config` |

> **关键教训**：`wsl.exe` 从 PowerShell 传参数给 bash 时会**吃掉 `$变量`**（如 `$svc`、`$KEY` 会变空）。凡是脚本里涉及变量，一律写成 `.sh` 文件再执行，不要用命令行内联传参。

---

## 二、阶段 1：核心概念实战（用项目现成 manifest 学）

### 2.1 关键命令

```bash
# 基础
kubectl get ns                                    # 查看 namespace（-A = 所有 namespace）
kubectl apply -f deploy/k8s/namespace.yaml        # 声明式创建
kubectl get pods -n tickethub -o wide             # -o wide 看 node/IP
kubectl describe pod <pod> -n tickethub           # 看详情 + Events（排障入口）
kubectl logs deployment/<svc> -n tickethub        # 看日志（注意：logs 要 pod 名或带资源前缀）
kubectl exec -it <pod> -n tickethub -- sh         # 进容器
kubectl port-forward svc/<svc> -n tickethub 18001:8001   # 本地访问集群内服务

# 自愈/滚动更新
kubectl delete pod <pod> -n tickethub             # 删 Pod → Deployment 自动重建（自愈）
kubectl rollout status deployment/<svc> -n tickethub     # 看滚动更新进度
kubectl rollout undo deployment/<svc> -n tickethub       # 回滚（旧 RS 保留的意义）
kubectl scale deployment/<svc> -n tickethub --replicas=5 # 扩缩容（Service 不用动）

# 调试
kubectl get events -n tickethub --sort-by=.lastTimestamp  # 全局事件
```

### 2.2 核心认知（踩坑中建立的）

- **铁三角**：Deployment（管副本/自愈/滚动）+ Service（管稳定访问）+ label/selector（关联两者）。`replicas: 2` 是 2 个 **Pod**，不是 2 个 RS；多次部署产生多个 RS（旧 RS 缩到 0，用于回滚）。
- **Service 分三层**：CoreDNS 把 `user-service` 解析成 ClusterIP → kube-proxy 把流量转发到具体 Pod IP。
- **kubectl create vs apply**：create 是命令式（已存在就报错），apply 是声明式（存在就更新），靠 `last-applied-configuration` annotation 做三方合并。
- **探针**：readiness 失败不重启（只是不给流量），liveness 失败重启（kubelet 杀掉容器）。
- **应用服务首次部署崩了**：`auth.jwt_secret is required` —— 项目安全设计（缺密钥拒绝启动）。解决：`kubectl create secret generic tickethub-secrets -n tickethub --from-env-file=.env` + Deployment 加 `envFrom: secretRef`。

### 2.3 业务镜像构建（网络受限环境的解法）

本机 Docker Hub 不通，但 **minikube 节点内网络能拉 Docker Hub/gcr.io/quay.io**。所以：

```bash
# 把 docker CLI 指向节点内 daemon（minikube docker-env）
eval $(minikube docker-env)

# 在节点内构建（基础镜像自动从 Docker Hub 拉）
./scripts/build-k8s-images.sh        # 循环构建 4 个服务镜像（已固化为脚本）
```

---

## 三、阶段 2：8 个中间件迁移全流程

### 3.1 迁移方法论（每个中间件都走这套）

1. 读 compose 定义：image / env / volumes / healthcheck / 网络依赖
2. 概念映射（compose → k8s）
3. 手写 manifest → `kubectl apply --dry-run=client -f x.yaml` 校验
4. `kubectl apply` → `kubectl get pods -w` 观察
5. 等价验证（redis-cli ping / mysqladmin ping / curl / kafka-topics）
6. 验证通过后清理 compose 对应服务

### 3.2 compose → k8s 概念映射表

| Docker Compose | Kubernetes | 注意 |
| --- | --- | --- |
| service | Deployment + Service | **Service 名 = compose 服务名** → 配置零改动 |
| `command:` | 优先 `args`（追加 ENTRYPOINT）；镜像无 ENTRYPOINT 时用 `command` 完整写 | `docker inspect <img> --format '{{.Config.Entrypoint}}'` 先查 |
| `ports:` | Service + port-forward | 端口冲突是常见坑（compose 还跑着时） |
| `environment:` | `env` / `envFrom` / `secretKeyRef` | 密钥走 Secret |
| `volumes: ./x:/y`（配置） | ConfigMap（单文件用 `subPath`） | 目录挂载会覆盖整个目录 |
| `volumes: named:/data`（数据） | PVC / volumeClaimTemplates（StatefulSet） | minikube 自带 standard StorageClass 自动建 PV |
| `depends_on: service_healthy` | readinessProbe / initContainers | k8s 没有 depends_on |
| `healthcheck:` | readinessProbe + livenessProbe | exec 探针注意 `timeoutSeconds` |
| `restart: always` | 不需要（Deployment 自愈） | |
| compose 网络内服务名 DNS | `<service>.<ns>.svc.cluster.local` | 集群内直接可用 |

### 3.3 各中间件迁移明细（含踩坑）

| 中间件 | manifest | 核心对象 | 遇到的问题 |
| --- | --- | --- | --- |
| **Redis** | `redis.yaml` | Deployment + Service + ConfigMap + PVC | 无坑（第一个练手，全程顺畅） |
| **MySQL** | `mysql.yaml` | StatefulSet + headless Service + volumeClaimTemplates | ① compose 的 `command: --character-set...` 写成 k8s `command` → 报 `exec: "--character-set-server": executable file not found` → 改用 **`args`**；② StatefulSet 更新后 Pod 不自动重建 → **`kubectl delete pod` 强制重建** |
| **Prometheus** | `observability.yaml` | Deployment + ConfigMap(subPath) | grafana 镜像拉取偶发 `EOF` → 节点内 `docker pull` 多轮重试 |
| **Grafana** | `observability.yaml` | Deployment + 4 个 ConfigMap + Secret + PVC | 同镜像问题；嵌套挂载（PVC 的 /var/lib/grafana 与 ConfigMap 的 /dashboards 共存） |
| **etcd** | `etcd.yaml` | StatefulSet + 普通 Service | **quay.io/coreos/etcd 镜像无 ENTRYPOINT**（`Entrypoint=[]`）→ `args` 被当可执行文件 → 必须 `command` 完整写（含 `/usr/local/bin/etcd`） |
| **Elasticsearch** | `elasticsearch.yaml` | StatefulSet | 镜像源 `docker.elastic.co` 下载极慢 → 5 轮重试；`vm.max_map_count` minikube 默认已满足（262144） |
| **Kafka** | `kafka.yaml` | StatefulSet + headless + `publishNotReadyAddresses: true` | ① **KRaft 自举死锁**：headless DNS 只解析 Ready Pod，而 Kafka 启动需要解析自己的名字 → 报 `UnknownHostException` → 加 `publishNotReadyAddresses: true`；② exec 探针默认 `timeout 1s`，JVM 启动要几秒 → 必超时 → `timeoutSeconds: 15` |
| **APISIX** | `apisix.yaml` | Deployment + ConfigMap + Secret | 路由初始化脚本默认 `TICKETHUB_APISIX_ADMIN_PORT=9181`（compose 的端口）→ 路由写进了 compose APISIX → 用 `APISIX_ADMIN=http://127.0.0.1:9180` 覆盖，`scripts/apisix-init-k8s.sh` 固化 |

### 3.4 验证命令（每个中间件的"体检"）

```bash
# 集群内验证（busybox-test 是常驻测试 Pod）
kubectl exec busybox-test -n tickethub -- wget -qO- http://tickethub-redis:6379    # 期望 +ERR（端口通）
kubectl exec tickethub-mysql-0 -n tickethub -- sh -c 'mysql -uroot -p$MYSQL_ROOT_PASSWORD -e "SHOW DATABASES;"'
kubectl exec busybox-test -n tickethub -- wget -qO- http://tickethub-etcd:2379/health   # {"health":"true"}
kubectl exec busybox-test -n tickethub -- wget -qO- http://tickethub-elasticsearch:9200/  # ES 版本信息
kubectl exec tickethub-kafka-0 -n tickethub -- /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --list
kubectl exec busybox-test -n tickethub -- wget -qO- http://tickethub-prometheus:9090/-/healthy
kubectl exec busybox-test -n tickethub -- wget -qO- http://tickethub-grafana:3000/api/health
# APISIX：port-forward 9180 后 curl admin API 列路由；9080 请求返回 502 说明路由链路通（upstream 未启动）
```

### 3.5 服务名同名的"红利"

k8s Service 名与 compose 服务名保持一致后：
- APISIX `config.yaml` 里 `http://tickethub-etcd:2379` → 集群内自动解析到 k8s etcd，**配置零改动**
- APISIX 路由的 `redis_host: tickethub-redis` → 自动连 k8s Redis
- Grafana datasource `url: http://tickethub-prometheus:9090` → 自动连 k8s Prometheus
- Prometheus 的 `tickethub-apisix:9091` target → APISIX 迁入后自动恢复抓取

---

## 四、常用命令速查

### kubectl
```bash
kubectl get <resource> [-n ns] [-o wide|yaml|json] [-w]   # 查
kubectl apply -f x.yaml                                   # 声明式应用
kubectl create <resource> ...                             # 命令式创建
kubectl delete <resource> <name> [-n ns]                  # 删除
kubectl describe <resource> <name> [-n ns]                # 详情 + Events
kubectl logs <pod|deployment/x> [-n ns] [--previous]      # 日志（--previous 看上次崩溃）
kubectl exec -it <pod> [-n ns] -- <cmd>                   # 进容器执行
kubectl port-forward svc/<name> [-n ns] 本地:远端          # 端口转发
kubectl rollout status/undo deployment/<name> [-n ns]     # 滚动更新状态/回滚
kubectl scale deployment/<name> [-n ns] --replicas=N      # 扩缩容
kubectl get secret <name> -n <ns> -o jsonpath='{.data.X}' | base64 -d   # 解密看 Secret
```

### minikube
```bash
minikube start / stop / status / delete        # 集群生命周期
minikube ssh -- "<cmd>"                        # 进节点执行（如 docker pull 预热镜像）
minikube docker-env                            # 输出指向节点 daemon 的环境变量
minikube dashboard                             # 可视化面板
```

### make（本项目已固化）
```bash
make k8s-start    # minikube start（含全部网络参数）
make k8s-stop     # minikube stop
make k8s-delete   # minikube delete（连数据一起删，慎用）
```

---

## 五、项目沉淀清单

| 文件 | 说明 |
| --- | --- |
| `deploy/k8s/namespace.yaml` | namespace |
| `deploy/k8s/tickethub-services.yaml` | 4 个应用服务（已加 envFrom Secret） |
| `deploy/k8s/redis.yaml` | Redis（Deployment） |
| `deploy/k8s/mysql.yaml` | MySQL（StatefulSet） |
| `deploy/k8s/observability.yaml` | Prometheus + Grafana |
| `deploy/k8s/etcd.yaml` | etcd（StatefulSet） |
| `deploy/k8s/elasticsearch.yaml` | ES（StatefulSet） |
| `deploy/k8s/kafka.yaml` | Kafka（StatefulSet + headless + publishNotReadyAddresses） |
| `deploy/k8s/apisix.yaml` | APISIX（Deployment） |
| `scripts/build-k8s-images.sh` | 节点内构建业务镜像 |
| `scripts/mysql-verify.sh` | MySQL 验证 |
| `scripts/apisix-init-k8s.sh` | APISIX 路由初始化（指向 k8s） |
| `scripts/apisix-verify.sh` | APISIX 验证 |
| `Makefile` | `k8s-start/stop/delete` target |
| `docs/k8s-learning-plan.md` | 学习计划（含本次迁移记录） |

---

## 六、遗留事项（下次继续）

1. ~~**应用入集群**（阶段 2.8）~~ ✅ 已完成（见第七节）：9 个 Go 服务全部部署进集群，`TICKETHUB_ADAPTER_MODE=infra` 全链路验证通过。Prometheus 的 `host.docker.internal:8001-8008` targets 尚未改为 Service DNS（待可观测性收尾）
2. **Helm 化**（阶段 3）：补全 `deploy/helm/tickethub`（ConfigMap/Secret/Ingress/HPA），中间件打包 chart
3. **可选项**：`.wslconfig` 内存提到 12GB（`wsl --shutdown` 生效）；WSL 默认用户改成 jkl；`sudo chown jkl:jkl .env deploy/k8s/redis.yaml`

---

## 七、应用入集群（阶段 2.8，2026-08-03 完成）

### 7.1 部署形态

- **9 个服务全量 manifest**：`deploy/k8s/tickethub-services.yaml`（重写为 9 个 Deployment + Service，含 envFrom Secret + 集群内地址 env）
- 镜像构建：`scripts/build-k8s-images.sh` 扩展到 9 个服务
- 服务间调用走集群内 Service DNS（gateway-bff 的 upstreams/grpc_upstreams 注入 `TICKETHUB_UPSTREAM_*` / `TICKETHUB_GRPC_UPSTREAM_*`）
- APISIX 路由 upstream 更新为 `gateway-bff:8080`（`scripts/apisix-init-k8s.sh` 已固化）

### 7.2 本次踩的坑

| # | 现象 | 根因 | 解决 |
| --- | --- | --- | --- |
| 1 | 4 个服务 CrashLoopBackOff（redis/kafka/grpc 地址为空） | **Go `os.ExpandEnv` 不支持 bash 的 `${VAR:-默认值}` 语法**——会把整个串当变量名，查不到替换成空 | 恢复 config.yaml 硬编码默认值，在 `pkg/config/config.go` 的 `ApplyDefaults` 加 env 覆盖逻辑（`TICKETHUB_REDIS_ADDR`、`TICKETHUB_KAFKA_BROKERS`、`TICKETHUB_ES_ADDRESSES`、`TICKETHUB_UPSTREAM_*`、`TICKETHUB_GRPC_UPSTREAM_*`） |
| 2 | `make k8s-start` 报 `.env: Permission denied` | `.env` 是 root 所有（600），Makefile `include .env` 失败 | `chown jkl:jkl .env` |
| 3 | git `dubious ownership` / `unlink Permission denied` | 之前 root 操作 + PowerShell 跨 WSL 写入导致文件所有者混乱 | `git config --global --add safe.directory` + `chown -R jkl:jkl app/ .git/` |
| 4 | root 的 kubectl 连不上（`127.0.0.1:2157 refused`） | minikube 重启后 API 端口变化，root 的 kubeconfig 过期 | 重新 `cp /home/jkl/.kube/config /root/.kube/config` |
| 5 | APISIX 路由 upstream 指向 WSL 主机 IP（172.31.125.198） | compose 时代 bootstrap 的默认 upstream | `BFF_UPSTREAM="gateway-bff:8080"` 重新 bootstrap |

### 7.3 验证结果（铁证）

- 9 服务 + 8 中间件 **21/21 Pod 全部 1/1 Running**
- `GET /healthz` 经 APISIX → **HTTP 200**
- `POST /api/users/register/captcha` 经 APISIX → **HTTP 200**
- Redis 中出现真实业务数据：`tickethub:user:register:captcha:*`（验证码）、`tickethub:user:register:bloom:*`（Bloom）、`tickethub:program:inventory:*`（库存）、`limit_req:*`（APISIX 限流）
- JWT 鉴权正常（未带 token 访问 /api/users/detail → 401）

### 7.4 经验沉淀

- **环境变量注入地址的正规姿势**：config.yaml 保持本地默认值，`ApplyDefaults` 里用 `os.Getenv` 覆盖（项目已有 `TICKETHUB_HTTP_ADDR` 等先例）；**不要**在 config.yaml 里用 `${VAR:-default}`（Go 的 ExpandEnv 不支持）
- 配置项要 env 化的判断标准：它在 k8s 里的值 ≠ 本地默认值（中间件地址、服务间调用地址）
- APISIX upstream node 格式是 `host:port`（无 scheme），scheme 由 `upstream.scheme` 字段控制
