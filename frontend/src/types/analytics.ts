export interface AnalyticsOverview {
  total_revenue: number
  total_invoices: number
  total_clients: number
  avg_invoice_value: number
}

export interface RevenueDataPoint {
  label: string
  total: number
  count: number
}

export interface RevenueResponse {
  period: "monthly"
  data: RevenueDataPoint[]
}

export interface TopClientData {
  client_name: string
  total: number
  count: number
}

export interface TopClientsResponse {
  clients: TopClientData[]
}

export interface TaxDataPoint {
  label: string
  tax: number
  revenue: number
}

export interface TaxSummaryResponse {
  data: TaxDataPoint[]
}
