DROP PROCEDURE IF EXISTS tickethub_order.add_order_query_indexes;

DELIMITER //
CREATE PROCEDURE tickethub_order.add_order_query_indexes(IN p_database_name VARCHAR(64), IN p_table_name VARCHAR(64))
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.statistics
    WHERE table_schema = p_database_name AND table_name = p_table_name AND index_name = 'idx_user_created_order'
  ) THEN
    SET @sql = CONCAT('CREATE INDEX idx_user_created_order ON `', p_database_name, '`.`', p_table_name, '` (user_id, created_at, order_number)');
    PREPARE statement FROM @sql;
    EXECUTE statement;
    DEALLOCATE PREPARE statement;
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.statistics
    WHERE table_schema = p_database_name AND table_name = p_table_name AND index_name = 'idx_user_status_created_order'
  ) THEN
    SET @sql = CONCAT('CREATE INDEX idx_user_status_created_order ON `', p_database_name, '`.`', p_table_name, '` (user_id, status, created_at, order_number)');
    PREPARE statement FROM @sql;
    EXECUTE statement;
    DEALLOCATE PREPARE statement;
  END IF;
END//
DELIMITER ;

CALL tickethub_order.add_order_query_indexes('tickethub_order', 'orders');
CALL tickethub_order.add_order_query_indexes('tickethub_order_0', 'orders_0');
CALL tickethub_order.add_order_query_indexes('tickethub_order_0', 'orders_1');
CALL tickethub_order.add_order_query_indexes('tickethub_order_1', 'orders_0');
CALL tickethub_order.add_order_query_indexes('tickethub_order_1', 'orders_1');
CALL tickethub_order.add_order_query_indexes('tickethub_order_2', 'orders_0');
CALL tickethub_order.add_order_query_indexes('tickethub_order_2', 'orders_1');

DROP PROCEDURE tickethub_order.add_order_query_indexes;
