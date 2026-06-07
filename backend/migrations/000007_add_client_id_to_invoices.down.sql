ALTER TABLE invoices DROP CONSTRAINT IF EXISTS fk_invoices_client;
ALTER TABLE invoices DROP COLUMN IF EXISTS client_id;
