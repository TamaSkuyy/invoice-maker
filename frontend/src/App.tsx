import { useState } from 'react'
import InvoiceForm from './components/InvoiceForm'
import InvoicePreview from './components/InvoicePreview'
import type { Invoice } from './types/invoice'

export default function App() {
  const [savedInvoices, setSavedInvoices] = useState<Invoice[]>([])
  const [preview, setPreview] = useState<Invoice | null>(null)

  const handleSaved = (invoice: Invoice) => {
    setSavedInvoices((prev) => [invoice, ...prev])
    setPreview(invoice)
  }

  return (
    <div className="min-h-screen bg-gray-50">
      {/* Navbar */}
      <header className="bg-white shadow-sm">
        <div className="mx-auto max-w-6xl px-4 py-4 flex items-center gap-3">
          <span className="text-2xl">🧾</span>
          <h1 className="text-xl font-bold text-blue-600">Invoice Maker</h1>
        </div>
      </header>

      <main className="mx-auto max-w-6xl px-4 py-8 grid grid-cols-1 lg:grid-cols-2 gap-8">
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
                <button
                  onClick={() => window.print()}
                  className="rounded border border-blue-500 px-3 py-1 text-sm text-blue-500 hover:bg-blue-50"
                >
                  🖨 Print
                </button>
              </div>
              <InvoicePreview invoice={preview} />
            </div>
          ) : (
            <div className="flex h-64 items-center justify-center rounded-xl border-2 border-dashed border-gray-200 text-gray-400">
              <p>Fill in the form and save to preview your invoice here.</p>
            </div>
          )}
        </section>
      </main>

      {/* Saved Invoices List */}
      {savedInvoices.length > 0 && (
        <section className="mx-auto max-w-6xl px-4 pb-12">
          <h2 className="mb-4 text-xl font-semibold text-gray-700">
            Saved Invoices
          </h2>
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
                    <td className="px-4 py-3 font-medium">{inv.client_name}</td>
                    <td className="px-4 py-3 text-gray-500">{inv.date}</td>
                    <td className="px-4 py-3 text-right font-mono font-semibold text-blue-600">
                      ${(inv.total_amount ?? 0).toFixed(2)}
                    </td>
                    <td className="px-4 py-3 text-right">
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
    </div>
  )
}
