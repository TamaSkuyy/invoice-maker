CREATE TABLE IF NOT EXISTS invoices (
    id UUID PRIMARY KEY,
    client_name VARCHAR(255) NOT NULL,
    date DATE NOT NULL,
    tax_rate DECIMAL(5, 2) NOT NULL DEFAULT 0,
    total_amount DECIMAL(10, 2) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_invoices_client_name ON invoices(client_name);
CREATE INDEX IF NOT EXISTS idx_invoices_date ON invoices(date);
