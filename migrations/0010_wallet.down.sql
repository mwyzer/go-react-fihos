BEGIN;

DROP INDEX IF EXISTS uq_payment_topup;
ALTER TABLE payments DROP CONSTRAINT IF EXISTS fk_payment_customer;
ALTER TABLE payments DROP COLUMN IF EXISTS customer_id;
ALTER TABLE payments DROP COLUMN IF EXISTS entity_type;

DROP TABLE IF EXISTS wallet_transactions;
ALTER TABLE customers DROP COLUMN IF EXISTS balance;

COMMIT;