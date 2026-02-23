import type { Invoice } from '../types/invoice'

interface Props {
  invoice: Invoice
}

export default function InvoicePreview({ invoice }: Props) {
  const subtotal = invoice.items.reduce(
    (acc, it) => acc + it.qty * it.price,
    0,
  )
  const taxAmount = subtotal * (invoice.tax_rate / 100)
  const total = invoice.total_amount ?? subtotal + taxAmount

  return (
    <div className="bg-white shadow-lg rounded-lg p-8 font-sans text-gray-800 print:shadow-none">
      {/* Header */}
      <div className="flex justify-between items-start mb-8">
        <div>
          <h1 className="text-3xl font-bold text-blue-600 tracking-wide">
            INVOICE
          </h1>
          {invoice.id && (
            <p className="text-sm text-gray-500 mt-1">#{invoice.id}</p>
          )}
        </div>
        <div className="text-right text-sm text-gray-600">
          <p className="font-semibold text-gray-800">Invoice Maker</p>
          <p>Date: {invoice.date}</p>
        </div>
      </div>

      {/* Bill To */}
      <div className="mb-6">
        <p className="text-xs uppercase font-semibold text-gray-400 mb-1">
          Bill To
        </p>
        <p className="text-lg font-semibold">{invoice.client_name}</p>
      </div>

      {/* Items Table */}
      <table className="w-full text-sm mb-6">
        <thead>
          <tr className="bg-blue-600 text-white">
            <th className="px-4 py-2 text-left rounded-tl">Description</th>
            <th className="px-4 py-2 text-right w-20">Qty</th>
            <th className="px-4 py-2 text-right w-28">Unit Price</th>
            <th className="px-4 py-2 text-right w-28 rounded-tr">Amount</th>
          </tr>
        </thead>
        <tbody>
          {invoice.items.map((item, idx) => (
            <tr
              key={idx}
              className={idx % 2 === 0 ? 'bg-gray-50' : 'bg-white'}
            >
              <td className="px-4 py-2">{item.description}</td>
              <td className="px-4 py-2 text-right font-mono">{item.qty}</td>
              <td className="px-4 py-2 text-right font-mono">
                ${item.price.toFixed(2)}
              </td>
              <td className="px-4 py-2 text-right font-mono">
                ${(item.qty * item.price).toFixed(2)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>

      {/* Totals */}
      <div className="flex justify-end">
        <div className="w-56 space-y-1 text-sm">
          <div className="flex justify-between text-gray-500">
            <span>Subtotal</span>
            <span className="font-mono">${subtotal.toFixed(2)}</span>
          </div>
          <div className="flex justify-between text-gray-500">
            <span>Tax ({invoice.tax_rate}%)</span>
            <span className="font-mono">${taxAmount.toFixed(2)}</span>
          </div>
          <div className="flex justify-between border-t border-gray-300 pt-2 font-bold text-base">
            <span>Total</span>
            <span className="font-mono text-blue-600">${total.toFixed(2)}</span>
          </div>
        </div>
      </div>

      {/* Footer */}
      <div className="mt-10 border-t border-gray-200 pt-4 text-center text-xs text-gray-400">
        Thank you for your business!
      </div>
    </div>
  )
}
