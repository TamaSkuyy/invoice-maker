import { useState, useEffect, useCallback } from "react";
import { Navbar } from "./Navbar";
import InvoiceForm from "./InvoiceForm";
import InvoicePreview from "./InvoicePreview";
import { DashboardCards } from "./DashboardCards";
import { lazy, Suspense } from "react";

// Lazy-load chart components — `recharts` is a heavy dependency (~150KB gzip).
// User yang cuma bikin invoice (tanpa buka Analytics tab) gak perlu download recharts.
const RevenueChart = lazy(() =>
  import("./RevenueChart").then((m) => ({ default: m.RevenueChart })),
);
const TopClientsChart = lazy(() =>
  import("./TopClientsChart").then((m) => ({ default: m.TopClientsChart })),
);

// Chart skeleton — ditampilkan saat chart component lagi di-load.
function ChartSkeleton() {
  return (
    <div className="rounded-xl border border-gray-100 bg-white p-6 shadow-sm">
      <div className="h-64 flex items-center justify-center">
        <div className="animate-spin h-6 w-6 border-2 border-green-500 border-t-transparent rounded-full" />
      </div>
    </div>
  );
}
import { TaxSummaryCard } from "./TaxSummaryCard";
import { apiFetch } from "../utils/api";
import { downloadFile } from "../utils/export";
import { User } from "../types/auth";
import type { Invoice } from "../types/invoice";
import type {
  AnalyticsOverview,
  RevenueDataPoint,
  TopClientData,
  TaxDataPoint,
} from "../types/analytics";

interface ProtectedInvoiceDashboardProps {
  user: User | null;
  onLogout: () => void;
  onNavigateToSettings?: () => void;
}

const currentYear = new Date().getFullYear();

export function ProtectedInvoiceDashboard({
  user,
  onLogout,
  onNavigateToSettings,
}: ProtectedInvoiceDashboardProps) {
  // Legacy state
  const [savedInvoices, setSavedInvoices] = useState<Invoice[]>([]);
  const [preview, setPreview] = useState<Invoice | null>(null);
  const [loading, setLoading] = useState(true);
  const [exporting, setExporting] = useState<string | null>(null);

  // Analytics state
  const [analyticsLoading, setAnalyticsLoading] = useState(true);
  const [overview, setOverview] = useState<AnalyticsOverview | null>(null);
  const [revenue, setRevenue] = useState<RevenueDataPoint[]>([]);
  const [topClients, setTopClients] = useState<TopClientData[]>([]);
  const [taxSummary, setTaxSummary] = useState<TaxDataPoint[]>([]);
  const [selectedYear, setSelectedYear] = useState(currentYear);
  const [statusFilter, setStatusFilter] = useState("")

  // Fetch saved invoices
  useEffect(() => {
    const fetchInvoices = async () => {
      try {
        const url = `/invoices${statusFilter ? `?status=${statusFilter}` : ""}`
        const invoices = await apiFetch<Invoice[]>(url);
        setSavedInvoices(invoices || []);
      } catch (err) {
        console.error("Failed to fetch invoices:", err);
      } finally {
        setLoading(false);
      }
    };
    fetchInvoices();
  }, [statusFilter]);

  // Fetch analytics data
  const fetchAnalytics = useCallback(async (year: number) => {
    setAnalyticsLoading(true);
    try {
      const [ov, revResp, tcResp, taxResp] = await Promise.all([
        apiFetch<AnalyticsOverview>("/analytics/overview"),
        apiFetch<{ data: RevenueDataPoint[] }>(`/analytics/revenue?year=${year}`),
        apiFetch<{ clients: TopClientData[] }>("/analytics/top-clients?limit=5"),
        apiFetch<{ data: TaxDataPoint[] }>(`/analytics/tax-summary?year=${year}`),
      ]);
      setOverview(ov);
      setRevenue(revResp.data || []);
      setTopClients(tcResp.clients || []);
      setTaxSummary(taxResp.data || []);
    } catch (err) {
      console.error("Failed to fetch analytics:", err);
    } finally {
      setAnalyticsLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchAnalytics(selectedYear);
  }, [fetchAnalytics, selectedYear]);

  const handleSaved = (invoice: Invoice) => {
    setSavedInvoices((prev) => [invoice, ...prev]);
    setPreview(invoice);
  };

  const handleDownload = async (endpoint: string, filename: string, label: string) => {
    setExporting(label);
    try {
      await downloadFile(endpoint, filename);
    } catch (err) {
      console.error(`Failed to download ${label}:`, err);
    } finally {
      setExporting(null);
    }
  };

  const handleChangeStatus = async (invoiceId: string, newStatus: string) => {
    try {
      await apiFetch(`/invoices/${invoiceId}/status`, {
        method: "PUT",
        body: JSON.stringify({ status: newStatus }),
      })
      // Re-fetch invoices
      const url = `/invoices${statusFilter ? `?status=${statusFilter}` : ""}`
      const invoices = await apiFetch<Invoice[]>(url)
      setSavedInvoices(invoices || [])
    } catch (err) {
      console.error("Failed to change status:", err)
    }
  }

  const handleRecordPayment = async (invoiceId: string, amount: number, date: string, method: string) => {
    try {
      await apiFetch(`/invoices/${invoiceId}/payments`, {
        method: "POST",
        body: JSON.stringify({ amount, date, method }),
      })
      // Refresh invoices
      const url = `/invoices${statusFilter ? `?status=${statusFilter}` : ""}`
      const invoices = await apiFetch<Invoice[]>(url)
      setSavedInvoices(invoices || [])
    } catch (err) {
      console.error("Failed to record payment:", err)
    }
  }

  // Status Colors
  const STATUS_COLORS: Record<string, string> = {
    Draft: "bg-gray-100 text-gray-700",
    Sent: "bg-blue-100 text-blue-700",
    Paid: "bg-green-100 text-green-700",
    Overdue: "bg-red-100 text-red-700",
    Cancelled: "bg-gray-100 text-gray-400 line-through",
  }

  function StatusBadge({ status = "Draft" }: { status?: string }) {
    const currentStatus = status || "Draft"
    const colorClass = STATUS_COLORS[currentStatus] || STATUS_COLORS.Draft
    return (
      <span className={`px-2 py-0.5 rounded-full text-xs font-medium ${colorClass}`}>
        {currentStatus}
      </span>
    )
  }

  return (
    <div className="bg-gray-50 min-h-screen">
      <Navbar user={user} onLogout={onLogout} onNavigateToSettings={onNavigateToSettings} />
      <main className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-6 md:py-8">

        {/* ── Dashboard Section ─────────────────────────────────── */}
        <section className="mb-10">
          <h2 className="text-2xl font-bold text-gray-800 mb-6">Dashboard</h2>

          <DashboardCards data={overview} loading={analyticsLoading} />

          <div className="grid grid-cols-1 lg:grid-cols-3 gap-6 mb-6">
            <div className="lg:col-span-2">
              <Suspense fallback={<ChartSkeleton />}>
                <RevenueChart
                  data={revenue}
                  loading={analyticsLoading}
                  year={selectedYear}
                  onYearChange={setSelectedYear}
                />
              </Suspense>
            </div>
            <div className="lg:col-span-1">
              <Suspense fallback={<ChartSkeleton />}>
                <TopClientsChart data={topClients} loading={analyticsLoading} />
              </Suspense>
            </div>
          </div>

          <TaxSummaryCard data={taxSummary} loading={analyticsLoading} year={selectedYear} />
        </section>

        {/* ── Invoice Section ───────────────────────────────────── */}
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-8 mb-12">
          <section className="bg-white rounded-xl shadow-sm border border-gray-100 p-6">
            <InvoiceForm onSaved={handleSaved} />
          </section>

          <section>
            {preview ? (
              <div className="space-y-4">
                <div className="flex items-center justify-between">
                  <h2 className="text-lg font-semibold text-gray-700">
                    Invoice Preview
                  </h2>
                  <div className="flex items-center gap-2">
                    <button
                      onClick={() => handleDownload(`/invoices/${preview.id}/pdf`, `invoice-${preview.id!.slice(0, 8)}.pdf`, "PDF")}
                      disabled={exporting !== null}
                      className="rounded-lg border border-green-600 px-3 py-1.5 text-sm font-medium text-green-600 hover:bg-green-50 disabled:opacity-50 transition-colors"
                    >
                      {exporting === "PDF" ? "Downloading..." : "Download PDF"}
                    </button>
                    <button
                      onClick={() => handleDownload(`/invoices/${preview.id}/csv`, `invoice-${preview.id!.slice(0, 8)}.csv`, "CSV")}
                      disabled={exporting !== null}
                      className="rounded-lg border border-gray-300 px-3 py-1.5 text-sm text-gray-600 hover:bg-gray-50 disabled:opacity-50 transition-colors"
                    >
                      {exporting === "CSV" ? "Exporting..." : "CSV"}
                    </button>
                    <button
                      onClick={() => window.print()}
                      className="rounded-lg border border-gray-300 px-3 py-1.5 text-sm text-gray-600 hover:bg-gray-50 transition-colors"
                    >
                      Print
                    </button>
                  </div>
                </div>
                <InvoicePreview invoice={preview} />
              </div>
            ) : (
              <div className="flex h-64 items-center justify-center rounded-xl border-2 border-dashed border-gray-200 bg-white text-gray-400">
                <p className="text-sm">Fill in the form and save to preview your invoice here.</p>
              </div>
            )}
          </section>
        </div>

        {/* ── Saved Invoices ────────────────────────────────────── */}
        {!loading && savedInvoices.length > 0 && (
          <section>
            <div className="flex items-center justify-between mb-4">
              <h2 className="text-lg font-semibold text-gray-700">
                Saved Invoices
              </h2>
              <div className="flex items-center gap-3">
                <select
                  value={statusFilter}
                  onChange={(e) => setStatusFilter(e.target.value)}
                  className="rounded-lg border border-gray-300 px-3 py-2 text-sm text-gray-600 focus:outline-none focus:ring-2 focus:ring-green-500 focus:border-transparent"
                >
                  <option value="">All Status</option>
                  <option value="Draft">Draft</option>
                  <option value="Sent">Sent</option>
                  <option value="Paid">Paid</option>
                  <option value="Overdue">Overdue</option>
                  <option value="Cancelled">Cancelled</option>
                </select>
                <button
                  onClick={() => handleDownload("/invoices/export/excel", "invoices.xlsx", "Excel")}
                  disabled={exporting !== null}
                  className="rounded-lg bg-green-600 px-4 py-2 text-sm font-medium text-white hover:bg-green-700 disabled:opacity-50 transition-colors shadow-sm"
                >
                  {exporting === "Excel" ? "Exporting..." : "Export to Excel"}
                </button>
              </div>
            </div>
            <div className="overflow-x-auto rounded-xl border border-gray-100 bg-white shadow-sm">
              <table className="w-full text-sm">
                <thead className="bg-gray-50 text-xs uppercase text-gray-600 tracking-wide">
                  <tr>
                    <th className="px-4 py-3 text-left font-medium">ID</th>
                    <th className="px-4 py-3 text-left font-medium">Client</th>
                    <th className="px-4 py-3 text-left font-medium">Date</th>
                    <th className="px-4 py-3 text-right font-medium">Total</th>
                    <th className="px-4 py-3 text-center font-medium">Status</th>
                    <th className="px-4 py-3 font-medium">Actions</th>
                    <th className="px-4 py-3 font-medium">Payment</th>
                  </tr>
                </thead>
                <tbody>
                  {savedInvoices.map((inv) => (
                    <tr
                      key={inv.id}
                      className="border-t border-gray-100 hover:bg-gray-50 transition-colors"
                    >
                      <td className="px-4 py-3 font-mono text-xs text-gray-500">
                        {inv.id}
                      </td>
                      <td className="px-4 py-3 font-medium text-gray-800">
                        {inv.client_name}
                      </td>
                      <td className="px-4 py-3 text-gray-500">{inv.date}</td>
                      <td className="px-4 py-3 text-right font-mono font-semibold text-emerald-600">
                        Rp {(inv.total_amount ?? 0).toFixed(2)}
                      </td>
                      <td className="px-4 py-3 text-center">
                        <StatusBadge status={inv.status} />
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-1.5">
                          {inv.status === "Draft" && (
                            <button
                              onClick={() => inv.id && handleChangeStatus(inv.id, "Sent")}
                              className="text-xs px-2.5 py-1 bg-blue-500 text-white rounded-md hover:bg-blue-600 transition-colors"
                            >
                              Mark Sent
                            </button>
                          )}
                          {(inv.status === "Draft" || inv.status === "Sent") && (
                            <button
                              onClick={() => inv.id && handleChangeStatus(inv.id, "Cancelled")}
                              className="text-xs px-2.5 py-1 bg-gray-400 text-white rounded-md hover:bg-gray-500 transition-colors"
                            >
                              Cancel
                            </button>
                          )}
                          <button
                            onClick={() => inv.id && handleDownload(`/invoices/${inv.id}/pdf`, `invoice-${inv.id.slice(0, 8)}.pdf`, "PDF")}
                            disabled={exporting !== null}
                            className="text-xs font-medium text-green-600 hover:text-green-500 transition-colors"
                          >
                            PDF
                          </button>
                          <button
                            onClick={() => setPreview(inv)}
                            className="text-xs font-medium text-blue-500 hover:text-blue-400 transition-colors"
                          >
                            View
                          </button>
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <form
                          onSubmit={(e) => {
                            e.preventDefault()
                            const fd = new FormData(e.currentTarget)
                            const amount = Number(fd.get("amount") ?? "0")
                            const date = fd.get("date")?.toString() ?? ""
                            const method = fd.get("method")?.toString() ?? "Transfer"
                            if (inv.id) handleRecordPayment(inv.id, amount, date, method)
                          }}
                          className="flex items-center gap-1.5"
                        >
                          <input name="amount" type="number" placeholder="Amount" className="w-20 border border-gray-300 rounded-md px-1.5 py-1 text-xs focus:outline-none focus:ring-1 focus:ring-green-500" />
                          <input name="date" type="date" className="border border-gray-300 rounded-md px-1.5 py-1 text-xs focus:outline-none focus:ring-1 focus:ring-green-500" />
                          <select name="method" className="border border-gray-300 rounded-md px-1.5 py-1 text-xs focus:outline-none focus:ring-1 focus:ring-green-500">
                            <option>Transfer</option>
                            <option>Cash</option>
                            <option>Credit Card</option>
                            <option>Check</option>
                          </select>
                          <button type="submit" className="px-2 py-1 bg-green-500 text-white rounded-md text-xs font-medium hover:bg-green-600 transition-colors whitespace-nowrap">
                            Pay
                          </button>
                        </form>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </section>
        )}

        {loading && (
          <div className="text-center py-12 text-gray-500">Loading invoices...</div>
        )}
      </main>
    </div>
  );
}
