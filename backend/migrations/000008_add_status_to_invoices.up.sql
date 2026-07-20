ALTER TABLE invoices ADD COLUMN status VARCHAR(20) NOT NULL DEFAULT 'Draft';
ALTER TABLE invoices ADD COLUMN due_date DATE;
CREATE INDEX idx_invoices_status ON invoices(status);
CREATE INDEX idx_invoices_due_date ON invoices(due_date);

-- Backfill existing invoices with a reasonable due_date
UPDATE invoices SET due_date = date + INTERVAL '30 days' WHERE due_date IS NULL;