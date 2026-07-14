USE tickethub_user;

DROP PROCEDURE IF EXISTS contract_user_privacy;

DELIMITER //
CREATE PROCEDURE contract_user_privacy()
BEGIN
  DECLARE unencrypted_users BIGINT DEFAULT 0;
  DECLARE unencrypted_ticket_users BIGINT DEFAULT 0;

  SELECT COUNT(*) INTO unencrypted_users
  FROM users
  WHERE mobile_ciphertext IS NULL OR mobile_lookup IS NULL OR privacy_key_version IS NULL;

  SELECT COUNT(*) INTO unencrypted_ticket_users
  FROM ticket_users
  WHERE name_ciphertext IS NULL OR certificate_ciphertext IS NULL
     OR certificate_lookup IS NULL OR privacy_key_version IS NULL;

  IF unencrypted_users > 0 OR unencrypted_ticket_users > 0 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'privacy migration incomplete; refusing to drop plaintext columns';
  END IF;

  IF EXISTS (
    SELECT 1 FROM information_schema.statistics
    WHERE table_schema=DATABASE() AND table_name='users' AND index_name='mobile'
  ) THEN
    ALTER TABLE users DROP INDEX mobile;
  END IF;

  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema=DATABASE() AND table_name='users' AND column_name='mobile'
  ) THEN
    ALTER TABLE users DROP COLUMN mobile;
  END IF;
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema=DATABASE() AND table_name='users' AND column_name='email'
  ) THEN
    ALTER TABLE users DROP COLUMN email;
  END IF;
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema=DATABASE() AND table_name='ticket_users' AND column_name='name'
  ) THEN
    ALTER TABLE ticket_users DROP COLUMN name;
  END IF;
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema=DATABASE() AND table_name='ticket_users' AND column_name='certificate_no'
  ) THEN
    ALTER TABLE ticket_users DROP COLUMN certificate_no;
  END IF;
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema=DATABASE() AND table_name='ticket_users' AND column_name='mobile'
  ) THEN
    ALTER TABLE ticket_users DROP COLUMN mobile;
  END IF;

  ALTER TABLE users
    MODIFY mobile_ciphertext VARBINARY(512) NOT NULL,
    MODIFY mobile_lookup BINARY(32) NOT NULL,
    MODIFY privacy_key_version VARCHAR(32) NOT NULL;
  ALTER TABLE ticket_users
    MODIFY name_ciphertext VARBINARY(512) NOT NULL,
    MODIFY certificate_ciphertext VARBINARY(512) NOT NULL,
    MODIFY certificate_lookup BINARY(32) NOT NULL,
    MODIFY privacy_key_version VARCHAR(32) NOT NULL;
END//
DELIMITER ;

CALL contract_user_privacy();
DROP PROCEDURE contract_user_privacy;
