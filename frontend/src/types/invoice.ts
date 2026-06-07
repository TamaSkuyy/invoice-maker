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
  items: InvoiceItem[]
  tax_rate: number
  total_amount?: number
}

export interface Client {
  id?: string
  name: string
  email: string
  phone: string
  address: string
}

export interface Product {
  id?: string
  name: string
  description: string
  default_price: number
}
