#!/usr/bin/env bash
# 验证 k8s 里的 MySQL：init SQL 是否执行（建库建表）
set -euo pipefail
kubectl exec tickethub-mysql-0 -n tickethub -- sh -c \
  'mysql -uroot -p"$MYSQL_ROOT_PASSWORD" -e "SHOW DATABASES; USE tickethub; SHOW TABLES;" 2>/dev/null'
