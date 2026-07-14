USE tickethub_program;

INSERT INTO programs (id, title, city, place, show_time, status, created_at)
VALUES
  (10001, 'TicketHub Live 2027', 'Shanghai', 'Mercedes-Benz Arena', '2027-05-20 19:30:00.000', 'ON_SALE', CURRENT_TIMESTAMP(3)),
  (10002, 'TicketHub Music Festival', 'Beijing', 'National Stadium', '2027-06-12 18:00:00.000', 'ON_SALE', CURRENT_TIMESTAMP(3)),
  (10003, 'TicketHub Drama Night', 'Hangzhou', 'Hangzhou Grand Theatre', '2027-04-18 19:30:00.000', 'COMING_SOON', CURRENT_TIMESTAMP(3))
ON DUPLICATE KEY UPDATE
  title = VALUES(title),
  city = VALUES(city),
  place = VALUES(place),
  show_time = VALUES(show_time),
  status = VALUES(status);

INSERT INTO ticket_categories (id, program_id, name, price_cent, total, remain, sell_started)
VALUES
  (1, 10001, 'A Zone', 128000, 1000, 1000, 1),
  (2, 10001, 'B Zone', 88000, 2000, 2000, 1),
  (3, 10002, 'Festival Pass', 68000, 5000, 5000, 1),
  (4, 10003, 'Stalls', 58000, 800, 800, 0)
ON DUPLICATE KEY UPDATE
  program_id = VALUES(program_id),
  name = VALUES(name),
  price_cent = VALUES(price_cent),
  total = VALUES(total),
  remain = VALUES(remain),
  sell_started = VALUES(sell_started);

INSERT INTO seats (id, program_id, ticket_category_id, row_code, col_code, price_cent, status)
VALUES
  (100, 10001, 1, 'A', '01', 128000, 'no_sold'),
  (101, 10001, 1, 'A', '02', 128000, 'no_sold'),
  (102, 10001, 1, 'A', '03', 128000, 'no_sold'),
  (103, 10001, 1, 'A', '04', 128000, 'no_sold')
ON DUPLICATE KEY UPDATE
  ticket_category_id = VALUES(ticket_category_id),
  price_cent = VALUES(price_cent),
  status = VALUES(status);

USE tickethub_base;

INSERT INTO areas (id, parent_id, name, level, hot)
VALUES
  (1, 0, 'China', 1, 0),
  (310000, 1, 'Shanghai', 2, 1),
  (110000, 1, 'Beijing', 2, 1),
  (330000, 1, 'Zhejiang', 2, 1)
ON DUPLICATE KEY UPDATE
  parent_id = VALUES(parent_id),
  name = VALUES(name),
  level = VALUES(level),
  hot = VALUES(hot);

INSERT INTO channel_data (id, code, value)
VALUES
  (1, 'WEB', 'TicketHub Web'),
  (2, 'H5', 'TicketHub H5'),
  (3, 'ADMIN', 'TicketHub Admin')
ON DUPLICATE KEY UPDATE value = VALUES(value);

USE tickethub_migrate;

INSERT INTO shard_mappings (virtual_shard, physical_db, physical_table, write_mode, version, updated_at)
VALUES
  (0, 'tickethub_order_0', 'orders_0', 'PRIMARY_ONLY', 1, CURRENT_TIMESTAMP(3)),
  (1, 'tickethub_order_0', 'orders_1', 'PRIMARY_ONLY', 1, CURRENT_TIMESTAMP(3)),
  (2, 'tickethub_order_1', 'orders_0', 'PRIMARY_ONLY', 1, CURRENT_TIMESTAMP(3)),
  (3, 'tickethub_order_1', 'orders_1', 'PRIMARY_ONLY', 1, CURRENT_TIMESTAMP(3))
ON DUPLICATE KEY UPDATE
  physical_db = VALUES(physical_db),
  physical_table = VALUES(physical_table),
  write_mode = VALUES(write_mode),
  version = VALUES(version),
  updated_at = VALUES(updated_at);
