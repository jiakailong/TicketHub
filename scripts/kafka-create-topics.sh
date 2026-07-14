#!/usr/bin/env bash
set -euo pipefail

KAFKA_CONTAINER="${KAFKA_CONTAINER:-tickethub-kafka}"
BOOTSTRAP_SERVER="${BOOTSTRAP_SERVER:-localhost:9092}"

create_topic() {
  local topic="$1"
  docker exec "${KAFKA_CONTAINER}" /opt/kafka/bin/kafka-topics.sh \
    --bootstrap-server "${BOOTSTRAP_SERVER}" \
    --create \
    --if-not-exists \
    --topic "${topic}" \
    --partitions 6 \
    --replication-factor 1 >/dev/null
}

create_topic ticket_hub.create_order
create_topic ticket_hub.program_changed

echo "TicketHub Kafka topics are ready"
