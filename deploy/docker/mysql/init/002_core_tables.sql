USE tickethub_user;

CREATE TABLE IF NOT EXISTS users (
  id BIGINT PRIMARY KEY,
  mobile_ciphertext VARBINARY(512) NOT NULL,
  mobile_lookup BINARY(32) NOT NULL,
  password_hash VARCHAR(255) NOT NULL,
  email_ciphertext VARBINARY(512) NULL,
  email_lookup BINARY(32) NULL,
  privacy_key_version VARCHAR(32) NOT NULL,
  real_name_status VARCHAR(32) NOT NULL,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NULL,
  UNIQUE INDEX uk_users_mobile_lookup (mobile_lookup),
  UNIQUE INDEX uk_users_email_lookup (email_lookup)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS ticket_users (
  id BIGINT PRIMARY KEY,
  user_id BIGINT NOT NULL,
  name_ciphertext VARBINARY(512) NOT NULL,
  certificate_ciphertext VARBINARY(512) NOT NULL,
  certificate_lookup BINARY(32) NOT NULL,
  mobile_ciphertext VARBINARY(512) NULL,
  privacy_key_version VARCHAR(32) NOT NULL,
  created_at DATETIME(3) NOT NULL,
  INDEX idx_user_id (user_id),
  INDEX idx_ticket_users_certificate_lookup (certificate_lookup)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

USE tickethub_program;

CREATE TABLE IF NOT EXISTS programs (
  id BIGINT PRIMARY KEY,
  title VARCHAR(255) NOT NULL,
  city VARCHAR(64) NOT NULL,
  place VARCHAR(255) NOT NULL,
  show_time DATETIME(3) NOT NULL,
  status VARCHAR(32) NOT NULL,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NULL,
  INDEX idx_city_show_time (city, show_time)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS ticket_categories (
  id BIGINT PRIMARY KEY,
  program_id BIGINT NOT NULL,
  name VARCHAR(64) NOT NULL,
  price_cent BIGINT NOT NULL,
  total BIGINT NOT NULL,
  remain BIGINT NOT NULL,
  sell_started TINYINT(1) NOT NULL DEFAULT 0,
  INDEX idx_program_id (program_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS seats (
  id BIGINT PRIMARY KEY,
  program_id BIGINT NOT NULL,
  ticket_category_id BIGINT NOT NULL,
  row_code VARCHAR(16) NOT NULL,
  col_code VARCHAR(16) NOT NULL,
  price_cent BIGINT NOT NULL,
  status VARCHAR(32) NOT NULL,
  INDEX idx_program_ticket (program_id, ticket_category_id),
  UNIQUE KEY uk_program_position (program_id, row_code, col_code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS program_records (
  id BIGINT PRIMARY KEY,
  program_id BIGINT NOT NULL,
  identifier_id BIGINT NOT NULL,
  record_type VARCHAR(32) NOT NULL,
  payload JSON NOT NULL,
  handle_status VARCHAR(32) NOT NULL,
  created_at DATETIME(3) NOT NULL,
  INDEX idx_program_handle (program_id, handle_status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

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

USE tickethub_order;

CREATE TABLE IF NOT EXISTS orders (
  order_number BIGINT PRIMARY KEY,
  program_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  ticket_category_id BIGINT NULL,
  seat_ids JSON NULL,
  ticket_user_ids JSON NULL,
  amount_cent BIGINT NOT NULL,
  status VARCHAR(32) NOT NULL,
  created_at DATETIME(3) NOT NULL,
  paid_at DATETIME(3) NULL,
  canceled_at DATETIME(3) NULL,
  refunded_at DATETIME(3) NULL,
  reconciliation_status VARCHAR(32) NULL,
  INDEX idx_user_created (user_id, created_at),
  INDEX idx_user_created_order (user_id, created_at, order_number),
  INDEX idx_user_status_created_order (user_id, status, created_at, order_number),
  INDEX idx_program_id (program_id),
  INDEX idx_program_status_category (program_id, status, ticket_category_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS order_ticket_users (
  id BIGINT PRIMARY KEY,
  order_number BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  ticket_user_id BIGINT NOT NULL,
  program_id BIGINT NOT NULL,
  ticket_category_id BIGINT NOT NULL,
  seat_id BIGINT NOT NULL,
  seat_info VARCHAR(128) NOT NULL,
  price_cent BIGINT NOT NULL,
  reconciliation_status VARCHAR(32) NULL,
  INDEX idx_order_number (order_number),
  INDEX idx_user_id (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS order_programs (
  id BIGINT PRIMARY KEY,
  order_number BIGINT NOT NULL,
  program_id BIGINT NOT NULL,
  identifier_id BIGINT NOT NULL,
  INDEX idx_program_id (program_id),
  INDEX idx_order_number (order_number)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS order_records (
  id BIGINT PRIMARY KEY,
  order_number BIGINT NOT NULL,
  program_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  record_type VARCHAR(32) NOT NULL,
  reconciliation_status VARCHAR(32) NULL,
  created_at DATETIME(3) NOT NULL,
  INDEX idx_program_user (program_id, user_id),
  INDEX idx_order_number (order_number)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS discard_orders (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  program_id BIGINT NOT NULL,
  order_number BIGINT NOT NULL,
  user_id BIGINT NULL,
  ticket_category_id BIGINT NULL,
  seat_ids JSON NULL,
  ticket_user_ids JSON NULL,
  amount_cent BIGINT NOT NULL DEFAULT 0,
  reason VARCHAR(64) NOT NULL,
  detail TEXT NULL,
  status VARCHAR(32) NOT NULL DEFAULT 'PENDING',
  created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  retried_at DATETIME(3) NULL,
  INDEX idx_program_id (program_id),
  INDEX idx_order_number (order_number),
  INDEX idx_status_program (status, program_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

USE tickethub_pay;

CREATE TABLE IF NOT EXISTS payments (
  order_number BIGINT PRIMARY KEY,
  amount_cent BIGINT NOT NULL,
  channel VARCHAR(32) NOT NULL,
  status VARCHAR(32) NOT NULL,
  pay_url VARCHAR(1024) NULL,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS refunds (
  id BIGINT PRIMARY KEY,
  order_number BIGINT NOT NULL,
  amount_cent BIGINT NOT NULL,
  reason VARCHAR(255) NOT NULL,
  success TINYINT(1) NOT NULL DEFAULT 0,
  created_at DATETIME(3) NOT NULL,
  INDEX idx_order_number (order_number)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

USE tickethub_base;

CREATE TABLE IF NOT EXISTS areas (
  id BIGINT PRIMARY KEY,
  parent_id BIGINT NOT NULL,
  name VARCHAR(64) NOT NULL,
  level INT NOT NULL,
  hot TINYINT(1) NOT NULL DEFAULT 0,
  INDEX idx_parent_id (parent_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS channel_data (
  id BIGINT PRIMARY KEY,
  code VARCHAR(64) NOT NULL UNIQUE,
  value VARCHAR(255) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

USE tickethub_customize;

CREATE TABLE IF NOT EXISTS api_data (
  id BIGINT PRIMARY KEY,
  path VARCHAR(255) NOT NULL,
  method VARCHAR(16) NOT NULL,
  user_id BIGINT NULL,
  created_at DATETIME(3) NOT NULL,
  INDEX idx_path_created (path, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS message_records (
  id BIGINT PRIMARY KEY,
  message_id VARCHAR(128) NOT NULL,
  topic VARCHAR(128) NOT NULL,
  status VARCHAR(32) NOT NULL,
  reason TEXT NULL,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NULL,
  UNIQUE KEY uk_message_topic (message_id, topic)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS rules (
  id BIGINT PRIMARY KEY,
  name VARCHAR(128) NOT NULL,
  enabled TINYINT(1) NOT NULL DEFAULT 1,
  payload JSON NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

USE tickethub_migrate;

CREATE TABLE IF NOT EXISTS migration_tasks (
  id BIGINT PRIMARY KEY,
  virtual_shard INT NOT NULL,
  source_shard VARCHAR(128) NOT NULL,
  target_shard VARCHAR(128) NOT NULL,
  status VARCHAR(32) NOT NULL,
  batch_size INT NOT NULL,
  cursor_order_number BIGINT NOT NULL DEFAULT 0,
  copied_rows BIGINT NOT NULL DEFAULT 0,
  error_message TEXT NULL,
  created_at DATETIME(3) NOT NULL,
  updated_at DATETIME(3) NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS shard_mappings (
  virtual_shard INT PRIMARY KEY,
  physical_db VARCHAR(128) NOT NULL,
  physical_table VARCHAR(128) NOT NULL,
  shadow_db VARCHAR(128) NULL,
  shadow_table VARCHAR(128) NULL,
  write_mode VARCHAR(32) NOT NULL DEFAULT 'PRIMARY_ONLY',
  version BIGINT NOT NULL DEFAULT 1,
  updated_at DATETIME(3) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

USE tickethub_order_0;
CREATE TABLE IF NOT EXISTS orders_0 LIKE tickethub_order.orders;
CREATE TABLE IF NOT EXISTS orders_1 LIKE tickethub_order.orders;

USE tickethub_order_1;
CREATE TABLE IF NOT EXISTS orders_0 LIKE tickethub_order.orders;
CREATE TABLE IF NOT EXISTS orders_1 LIKE tickethub_order.orders;

USE tickethub_order_2;
CREATE TABLE IF NOT EXISTS orders_0 LIKE tickethub_order.orders;
CREATE TABLE IF NOT EXISTS orders_1 LIKE tickethub_order.orders;
