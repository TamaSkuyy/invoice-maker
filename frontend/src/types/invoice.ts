export interface InvoiceItem {
  description: string
  qty: number
  price: number
}

export interface Invoice {
  id?: string
  client_name: string
  client_id?: string | null
  date: string
  due_date?: string
  items: InvoiceItem[]
  tax_rate: number
  total_amount?: number
  status?: string
  user_id?: string
  created_at?: string
  updated_at?: string
}

export interface Client {
  id?: string
  user_id: string
  name: string
  email: string
  phone: string
  address: string
  created_at: string
  updated_at: string
}

export interface Product {
  id?: string
  user_id: string
  name: string
  description: string
  default_price: number
  created_at: string
  updated_at: string
}

export interface Payment {
  id: string
  invoice_id: string
  amount: number
  date: string
  method: string
  recorded_by: string
  created_at: string
}

export interface StatusHistoryEntry {
  id: string
  invoice_id: string
  old_status: string | null
  new_status: string
  changed_by: string
  changed_at: string
}
