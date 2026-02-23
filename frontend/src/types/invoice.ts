export interface InvoiceItem {
  description: string
  qty: number
  price: number
}

export interface Invoice {
  id?: string
  client_name: string
  date: string
  items: InvoiceItem[]
  tax_rate: number
  total_amount?: number
}
