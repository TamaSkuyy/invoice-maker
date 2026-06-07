ALTER TABLE invoices ADD COLUMN IF NOT EXISTS client_id UUID;
ALTER TABLE invoices ADD CONSTRAINT fk_invoices_client
  FOREIGN KEY (client_id) REFERENCES clients(id) ON DELETE SET NULL;
