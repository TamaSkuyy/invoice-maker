DROP INDEX idx_invoices_user_id_created;
ALTER TABLE invoices DROP CONSTRAINT fk_invoices_user_id;
ALTER TABLE invoices DROP COLUMN user_id;
