import { useState, useEffect } from "react";
import { Navbar } from "./Navbar";
import InvoiceForm from "./InvoiceForm";
import InvoicePreview from "./InvoicePreview";
import { apiFetch } from "../utils/api";
import { downloadFile } from "../utils/export";
import { User } from "../types/auth";
import type { Invoice } from "../types/invoice";

interface ProtectedInvoiceDashboardProps {
  user: User | null;
  onLogout: () => void;
}

export function ProtectedInvoiceDashboard({
  user,
  onLogout,
}: ProtectedInvoiceDashboardProps) {
  const [savedInvoices, setSavedInvoices] = useState<Invoice[]>([]);
  const [preview, setPreview] = useState<Invoice | null>(null);
  const [loading, setLoading] = useState(true);
  const [exporting, setExporting] = useState<string | null>(null);

  useEffect(() => {
    const fetchInvoices = async () => {
      try {
        const invoices = await apiFetch<Invoice[]>("/invoices");
        setSavedInvoices(invoices || []);
      } catch (err) {
        console.error("Failed to fetch invoices:", err);
      } finally {
        setLoading(false);
      }
    };

    fetchInvoices();
  }, []);

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

  return (
    <>
      <Navbar user={user} onLogout={onLogout} />
      <main className="max-w-6xl mx-auto px-4 py-8">
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-8 mb-12">
          {/* Form Panel */}
          <section className="bg-white rounded-xl shadow p-6">
            <InvoiceForm onSaved={handleSaved} />
          </section>

          {/* Preview Panel */}
          <section>
            {preview ? (
              <div className="space-y-4">
                <div className="flex items-center justify-between">
                  <h2 className="text-xl font-semibold text-gray-700">
                    Invoice Preview
                  </h2>
                  <div className="flex items-center gap-2">
                    <button
                      onClick={() => handleDownload(`/invoices/${preview.id}/pdf`, `invoice-${preview.id!.slice(0, 8)}.pdf`, "PDF")}
                      disabled={exporting !== null}
                      className="rounded border border-green-600 px-3 py-1 text-sm text-green-600 hover:bg-green-50 disabled:opacity-50"
                    >
                      {exporting === "PDF" ? "Downloading..." : "Download PDF"}
                    </button>
                    <button
                      onClick={() => handleDownload(`/invoices/${preview.id}/csv`, `invoice-${preview.id!.slice(0, 8)}.csv`, "CSV")}
                      disabled={exporting !== null}
                      className="rounded border border-gray-400 px-3 py-1 text-sm text-gray-600 hover:bg-gray-50 disabled:opacity-50"
                    >
                      {exporting === "CSV" ? "Exporting..." : "CSV"}
                    </button>
                    <button
                      onClick={() => window.print()}
                      className="rounded border border-blue-500 px-3 py-1 text-sm text-blue-500 hover:bg-blue-50"
                    >
                      Print
                    </button>
                  </div>
                </div>
                <InvoicePreview invoice={preview} />
              </div>
            ) : (
              <div className="flex h-64 items-center justify-center rounded-xl border-2 border-dashed border-gray-200 text-gray-400">
                <p>Fill in the form and save to preview your invoice here.</p>
              </div>
            )}
          </section>
        </div>

        {/* Saved Invoices List */}
        {!loading && savedInvoices.length > 0 && (
          <section>
            <div className="flex items-center justify-between mb-4">
              <h2 className="text-xl font-semibold text-gray-700">
                Saved Invoices
              </h2>
              <button
                onClick={() => handleDownload("/invoices/export/excel", "invoices.xlsx", "Excel")}
                disabled={exporting !== null}
                className="rounded border border-green-600 px-4 py-2 text-sm font-medium text-green-600 hover:bg-green-50 disabled:opacity-50"
              >
                {exporting === "Excel" ? "Exporting..." : "Export to Excel"}
              </button>
            </div>
            <div className="overflow-x-auto rounded-xl bg-white shadow">
              <table className="w-full text-sm">
                <thead className="bg-gray-50 text-xs uppercase text-gray-500">
                  <tr>
                    <th className="px-4 py-3 text-left">ID</th>
                    <th className="px-4 py-3 text-left">Client</th>
                    <th className="px-4 py-3 text-left">Date</th>
                    <th className="px-4 py-3 text-right">Total</th>
                    <th className="px-4 py-3"></th>
                  </tr>
                </thead>
                <tbody>
                  {savedInvoices.map((inv) => (
                    <tr
                      key={inv.id}
                      className="border-t border-gray-100 hover:bg-gray-50"
                    >
                      <td className="px-4 py-3 font-mono text-xs text-gray-500">
                        {inv.id}
                      </td>
                      <td className="px-4 py-3 font-medium">
                        {inv.client_name}
                      </td>
                      <td className="px-4 py-3 text-gray-500">{inv.date}</td>
                      <td className="px-4 py-3 text-right font-mono font-semibold text-blue-600">
                        ${(inv.total_amount ?? 0).toFixed(2)}
                      </td>
                      <td className="px-4 py-3 text-right space-x-2">
                        <button
                          onClick={() => handleDownload(`/invoices/${inv.id}/pdf`, `invoice-${inv.id!.slice(0, 8)}.pdf`, "PDF")}
                          disabled={exporting !== null}
                          className="text-green-600 hover:underline text-xs"
                        >
                          PDF
                        </button>
                        <button
                          onClick={() => setPreview(inv)}
                          className="text-blue-500 hover:underline"
                        >
                          View
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </section>
        )}

        {loading && (
          <div className="text-center text-gray-600">Loading invoices...</div>
        )}
      </main>
    </>
  );
}
