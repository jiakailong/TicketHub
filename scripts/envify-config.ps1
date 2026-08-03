# 将 9 个服务 config.yaml 中的硬编码 127.0.0.1 改为 ${ENV:-默认值} 形式，
# 使 k8s 部署可通过环境变量注入集群内 Service DNS，同时本地开发默认值不变。
# 用法: powershell -File scripts/envify-config.ps1
$ErrorActionPreference = "Stop"

$repo = Split-Path -Parent $PSScriptRoot

# 通用替换（所有服务）：redis / kafka
function Replace-InFile($relPath, $pairs) {
    $p = Join-Path $repo $relPath
    $content = [IO.File]::ReadAllText($p)
    foreach ($pair in $pairs) {
        if (-not $content.Contains($pair[0])) {
            Write-Warning "$relPath : 未找到 '$($pair[0].Trim())'"
            continue
        }
        $content = $content.Replace($pair[0], $pair[1])
    }
    [IO.File]::WriteAllText($p, $content)
    Write-Host "updated: $relPath"
}

$common = @(
    ,@('addr: 127.0.0.1:6379', 'addr: ${TICKETHUB_REDIS_ADDR:-127.0.0.1:6379}')
    ,@('brokers: [127.0.0.1:9094]', 'brokers: [${TICKETHUB_KAFKA_BROKERS:-127.0.0.1:9094}]')
)

$all = @(
    'user-service','program-service','order-service','pay-service',
    'base-data-service','customize-service','admin-service','migrate-service','gateway-bff'
)

foreach ($svc in $all) {
    Replace-InFile "app/$svc/configs/config.yaml" $common
}

# gateway-bff: upstreams + grpc_upstreams + trusted_proxy_cidrs
$gw = @(
    ,@('  user-service: http://127.0.0.1:8001', '  user-service: ${TICKETHUB_UPSTREAM_USER_SERVICE:-http://127.0.0.1:8001}')
    ,@('  program-service: http://127.0.0.1:8002', '  program-service: ${TICKETHUB_UPSTREAM_PROGRAM_SERVICE:-http://127.0.0.1:8002}')
    ,@('  order-service: http://127.0.0.1:8003', '  order-service: ${TICKETHUB_UPSTREAM_ORDER_SERVICE:-http://127.0.0.1:8003}')
    ,@('  pay-service: http://127.0.0.1:8004', '  pay-service: ${TICKETHUB_UPSTREAM_PAY_SERVICE:-http://127.0.0.1:8004}')
    ,@('  base-data-service: http://127.0.0.1:8005', '  base-data-service: ${TICKETHUB_UPSTREAM_BASE_DATA_SERVICE:-http://127.0.0.1:8005}')
    ,@('  customize-service: http://127.0.0.1:8006', '  customize-service: ${TICKETHUB_UPSTREAM_CUSTOMIZE_SERVICE:-http://127.0.0.1:8006}')
    ,@('  admin-service: http://127.0.0.1:8007', '  admin-service: ${TICKETHUB_UPSTREAM_ADMIN_SERVICE:-http://127.0.0.1:8007}')
    ,@('  migrate-service: http://127.0.0.1:8008', '  migrate-service: ${TICKETHUB_UPSTREAM_MIGRATE_SERVICE:-http://127.0.0.1:8008}')
    ,@('  user-service: 127.0.0.1:9001', '  user-service: ${TICKETHUB_GRPC_UPSTREAM_USER_SERVICE:-127.0.0.1:9001}')
    ,@('  program-service: 127.0.0.1:9002', '  program-service: ${TICKETHUB_GRPC_UPSTREAM_PROGRAM_SERVICE:-127.0.0.1:9002}')
    ,@('  order-service: 127.0.0.1:9003', '  order-service: ${TICKETHUB_GRPC_UPSTREAM_ORDER_SERVICE:-127.0.0.1:9003}')
    ,@('  pay-service: 127.0.0.1:9004', '  pay-service: ${TICKETHUB_GRPC_UPSTREAM_PAY_SERVICE:-127.0.0.1:9004}')
    ,@('  base-data-service: 127.0.0.1:9005', '  base-data-service: ${TICKETHUB_GRPC_UPSTREAM_BASE_DATA_SERVICE:-127.0.0.1:9005}')
    ,@('  customize-service: 127.0.0.1:9006', '  customize-service: ${TICKETHUB_GRPC_UPSTREAM_CUSTOMIZE_SERVICE:-127.0.0.1:9006}')
    ,@('  admin-service: 127.0.0.1:9007', '  admin-service: ${TICKETHUB_GRPC_UPSTREAM_ADMIN_SERVICE:-127.0.0.1:9007}')
    ,@('  migrate-service: 127.0.0.1:9008', '  migrate-service: ${TICKETHUB_GRPC_UPSTREAM_MIGRATE_SERVICE:-127.0.0.1:9008}')
    ,@("    - 172.16.0.0/12`n", "    - 172.16.0.0/12`n    - 10.0.0.0/8`n")
)
Replace-InFile 'app/gateway-bff/configs/config.yaml' $gw

# order-service: grpc_upstreams
Replace-InFile 'app/order-service/configs/config.yaml' @(
    ,@('  program-service: 127.0.0.1:9002', '  program-service: ${TICKETHUB_GRPC_UPSTREAM_PROGRAM_SERVICE:-127.0.0.1:9002}')
    ,@('  migrate-service: 127.0.0.1:9008', '  migrate-service: ${TICKETHUB_GRPC_UPSTREAM_MIGRATE_SERVICE:-127.0.0.1:9008}')
)

# pay-service / admin-service: grpc_upstreams order-service
$payAdmin = @(
    ,@('  order-service: 127.0.0.1:9003', '  order-service: ${TICKETHUB_GRPC_UPSTREAM_ORDER_SERVICE:-127.0.0.1:9003}')
)
Replace-InFile 'app/pay-service/configs/config.yaml' $payAdmin
Replace-InFile 'app/admin-service/configs/config.yaml' $payAdmin

# program-service: grpc_upstreams user-service + elasticsearch
$prog = @(
    ,@('  user-service: 127.0.0.1:9001', '  user-service: ${TICKETHUB_GRPC_UPSTREAM_USER_SERVICE:-127.0.0.1:9001}')
    ,@('  addresses: [http://127.0.0.1:9200]', '  addresses: [${TICKETHUB_ES_ADDRESSES:-http://127.0.0.1:9200}]')
)
Replace-InFile 'app/program-service/configs/config.yaml' $prog

# user-service: trusted_proxy_cidrs
Replace-InFile 'app/user-service/configs/config.yaml' @(
    ,@("    - 172.16.0.0/12`n", "    - 172.16.0.0/12`n    - 10.0.0.0/8`n")
)

Write-Host "done."
