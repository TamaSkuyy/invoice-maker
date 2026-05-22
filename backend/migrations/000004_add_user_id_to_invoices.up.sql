ALTER TABLE invoices ADD COLUMN user_id UUID;
ALTER TABLE invoices ADD CONSTRAINT fk_invoices_user_id
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
CREATE INDEX idx_invoices_user_id_created ON invoices(user_id, created_at DESC);
