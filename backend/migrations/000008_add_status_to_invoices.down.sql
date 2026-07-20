DROP INDEX IF EXISTS idx_invoices_due_date;
DROP INDEX IF EXISTS idx_invoices_status;
ALTER TABLE invoices DROP COLUMN due_date;
ALTER TABLE invoices DROP COLUMN status;