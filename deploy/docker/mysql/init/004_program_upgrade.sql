USE tickethub_program;

CREATE TABLE IF NOT EXISTS program_outbox (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  event_id VARCHAR(64) NOT NULL,
  topic VARCHAR(128) NOT NULL,
  event_key VARCHAR(128) NOT NULL,
  trace_id VARCHAR(128) NULL,
  schema_version VARCHAR(32) NOT NULL,
  occurred_at DATETIME(3) NOT NULL,
  payload JSON NOT NULL,
  status VARCHAR(32) NOT NULL,
  attempts INT NOT NULL DEFAULT 0,
  available_at DATETIME(3) NOT NULL,
  lease_until DATETIME(3) NULL,
  last_error VARCHAR(1000) NULL,
  created_at DATETIME(3) NOT NULL,
  published_at DATETIME(3) NULL,
  UNIQUE KEY uk_program_outbox_event (event_id),
  INDEX idx_program_outbox_pending (status, available_at, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
