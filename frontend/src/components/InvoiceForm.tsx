import { useState, useCallback } from 'react'
import type { Invoice, InvoiceItem } from '../types/invoice'

const EMPTY_ITEM: InvoiceItem = { description: '', qty: 1, price: 0 }

interface Props {
  onSaved: (invoice: Invoice) => void
}

export default function InvoiceForm({ onSaved }: Props) {
  const today = new Date().toISOString().split('T')[0]
  const [clientName, setClientName] = useState('')
  const [date, setDate] = useState(today)
  const [items, setItems] = useState<InvoiceItem[]>([{ ...EMPTY_ITEM }])
  const [taxRate, setTaxRate] = useState(10)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const subtotal = items.reduce((acc, it) => acc + it.qty * it.price, 0)
  const taxAmount = subtotal * (taxRate / 100)
  const grandTotal = subtotal + taxAmount

  const updateItem = useCallback(
    (index: number, field: keyof InvoiceItem, value: string | number) => {
      setItems((prev) => {
        const next = [...prev]
        next[index] = { ...next[index]!, [field]: value } as InvoiceItem
        return next
      })
    },
    [],
  )

  const addItem = () => setItems((prev) => [...prev, { ...EMPTY_ITEM }])

  const removeItem = (index: number) =>
    setItems((prev) => prev.filter((_, i) => i !== index))

  const handleSave = async () => {
    setError(null)
    if (!clientName.trim()) {
      setError('Client name is required.')
      return
    }
    if (items.some((it) => !it.description.trim())) {
      setError('All line items must have a description.')
      return
    }
    const payload: Invoice = {
      client_name: clientName,
      date,
      items,
      tax_rate: taxRate,
    }
    setSaving(true)
    try {
      const res = await fetch('/api/invoices', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      })
      if (!res.ok) {
        const body = (await res.json()) as { error?: string }
        throw new Error(body.error ?? 'Unknown error')
      }
      const saved = (await res.json()) as Invoice
      onSaved(saved)
      setClientName('')
      setDate(today)
      setItems([{ ...EMPTY_ITEM }])
      setTaxRate(10)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save invoice.')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="space-y-6">
      <h2 className="text-xl font-semibold text-gray-700">New Invoice</h2>

      {error && (
        <div className="rounded bg-red-50 p-3 text-sm text-red-600">{error}</div>
      )}

      {/* Client & Date */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <div>
          <label className="block text-sm font-medium text-gray-600">
            Client Name
          </label>
          <input
            className="mt-1 w-full rounded border border-gray-300 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-400"
            value={clientName}
            onChange={(e) => setClientName(e.target.value)}
            placeholder="Acme Corp"
          />
        </div>
        <div>
          <label className="block text-sm font-medium text-gray-600">Date</label>
          <input
            type="date"
            className="mt-1 w-full rounded border border-gray-300 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-400"
            value={date}
            onChange={(e) => setDate(e.target.value)}
          />
        </div>
      </div>

      {/* Line Items */}
      <div>
        <label className="block text-sm font-medium text-gray-600 mb-2">
          Line Items
        </label>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="bg-gray-100 text-left text-xs uppercase text-gray-500">
                <th className="px-3 py-2">Description</th>
                <th className="px-3 py-2 w-20">Qty</th>
                <th className="px-3 py-2 w-28">Unit Price</th>
                <th className="px-3 py-2 w-28 text-right">Amount</th>
                <th className="px-3 py-2 w-10"></th>
              </tr>
            </thead>
            <tbody>
              {items.map((item, idx) => (
                <tr key={idx} className="border-b border-gray-100">
                  <td className="px-3 py-1">
                    <input
                      className="w-full rounded border border-gray-200 px-2 py-1 focus:outline-none focus:ring-1 focus:ring-blue-300"
                      value={item.description}
                      onChange={(e) =>
                        updateItem(idx, 'description', e.target.value)
                      }
                      placeholder="Service / Product"
                    />
                  </td>
                  <td className="px-3 py-1">
                    <input
                      type="number"
                      min={1}
                      className="w-full rounded border border-gray-200 px-2 py-1 focus:outline-none focus:ring-1 focus:ring-blue-300"
                      value={item.qty}
                      onChange={(e) =>
                        updateItem(idx, 'qty', parseFloat(e.target.value) || 0)
                      }
                    />
                  </td>
                  <td className="px-3 py-1">
                    <input
                      type="number"
                      min={0}
                      step={0.01}
                      className="w-full rounded border border-gray-200 px-2 py-1 focus:outline-none focus:ring-1 focus:ring-blue-300"
                      value={item.price}
                      onChange={(e) =>
                        updateItem(idx, 'price', parseFloat(e.target.value) || 0)
                      }
                    />
                  </td>
                  <td className="px-3 py-1 text-right font-mono">
                    ${(item.qty * item.price).toFixed(2)}
                  </td>
                  <td className="px-3 py-1 text-center">
                    {items.length > 1 && (
                      <button
                        onClick={() => removeItem(idx)}
                        className="text-red-400 hover:text-red-600"
                        title="Remove item"
                      >
                        ✕
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <button
          onClick={addItem}
          className="mt-2 text-sm text-blue-500 hover:underline"
        >
          + Add line item
        </button>
      </div>

      {/* Tax & Totals */}
      <div className="flex justify-end">
        <div className="w-64 space-y-1 text-sm">
          <div className="flex justify-between">
            <span className="text-gray-500">Subtotal</span>
            <span className="font-mono">${subtotal.toFixed(2)}</span>
          </div>
          <div className="flex items-center justify-between">
            <label className="text-gray-500">
              Tax&nbsp;
              <input
                type="number"
                min={0}
                max={100}
                step={0.5}
                className="w-14 rounded border border-gray-200 px-1 py-0.5 text-center font-mono focus:outline-none focus:ring-1 focus:ring-blue-300"
                value={taxRate}
                onChange={(e) =>
                  setTaxRate(parseFloat(e.target.value) || 0)
                }
              />
              %
            </label>
            <span className="font-mono">${taxAmount.toFixed(2)}</span>
          </div>
          <div className="flex justify-between border-t border-gray-200 pt-1 font-semibold">
            <span>Grand Total</span>
            <span className="font-mono text-blue-600">${grandTotal.toFixed(2)}</span>
          </div>
        </div>
      </div>

      <button
        onClick={handleSave}
        disabled={saving}
        className="w-full rounded bg-blue-600 py-2 text-white font-semibold hover:bg-blue-700 disabled:opacity-50 transition-colors"
      >
        {saving ? 'Saving…' : 'Save Invoice'}
      </button>
    </div>
  )
}
