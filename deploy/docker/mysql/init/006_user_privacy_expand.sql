USE tickethub_user;

DROP PROCEDURE IF EXISTS ensure_privacy_column;
DROP PROCEDURE IF EXISTS ensure_privacy_index;

DELIMITER //
CREATE PROCEDURE ensure_privacy_column(IN p_table VARCHAR(64), IN p_column VARCHAR(64), IN p_definition VARCHAR(255))
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = p_table AND column_name = p_column
  ) THEN
    SET @sql = CONCAT('ALTER TABLE `', p_table, '` ADD COLUMN `', p_column, '` ', p_definition);
    PREPARE statement FROM @sql;
    EXECUTE statement;
    DEALLOCATE PREPARE statement;
  END IF;
END//

CREATE PROCEDURE ensure_privacy_index(IN p_table VARCHAR(64), IN p_index VARCHAR(64), IN p_kind VARCHAR(32), IN p_columns VARCHAR(255))
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = p_table AND index_name = p_index
  ) THEN
    SET @sql = CONCAT('CREATE ', p_kind, ' `', p_index, '` ON `', p_table, '` ', p_columns);
    PREPARE statement FROM @sql;
    EXECUTE statement;
    DEALLOCATE PREPARE statement;
  END IF;
END//
DELIMITER ;

CALL ensure_privacy_column('users', 'mobile_ciphertext', 'VARBINARY(512) NULL');
CALL ensure_privacy_column('users', 'mobile_lookup', 'BINARY(32) NULL');
CALL ensure_privacy_column('users', 'email_ciphertext', 'VARBINARY(512) NULL');
CALL ensure_privacy_column('users', 'email_lookup', 'BINARY(32) NULL');
CALL ensure_privacy_column('users', 'privacy_key_version', 'VARCHAR(32) NULL');
CALL ensure_privacy_index('users', 'uk_users_mobile_lookup', 'UNIQUE INDEX', '(`mobile_lookup`)');
CALL ensure_privacy_index('users', 'uk_users_email_lookup', 'UNIQUE INDEX', '(`email_lookup`)');

CALL ensure_privacy_column('ticket_users', 'name_ciphertext', 'VARBINARY(512) NULL');
CALL ensure_privacy_column('ticket_users', 'certificate_ciphertext', 'VARBINARY(512) NULL');
CALL ensure_privacy_column('ticket_users', 'certificate_lookup', 'BINARY(32) NULL');
CALL ensure_privacy_column('ticket_users', 'mobile_ciphertext', 'VARBINARY(512) NULL');
CALL ensure_privacy_column('ticket_users', 'privacy_key_version', 'VARCHAR(32) NULL');
CALL ensure_privacy_index('ticket_users', 'idx_ticket_users_certificate_lookup', 'INDEX', '(`certificate_lookup`)');

DROP PROCEDURE ensure_privacy_column;
DROP PROCEDURE ensure_privacy_index;
