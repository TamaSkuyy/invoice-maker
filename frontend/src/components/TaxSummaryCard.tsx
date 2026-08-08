import { useState } from "react";
import { downloadFile } from "../utils/export";
import type { TaxDataPoint } from "../types/analytics";

interface TaxSummaryCardProps {
  data: TaxDataPoint[];
  loading: boolean;
  year: number;
}

function formatIDR(value: number): string {
  if (value >= 1_000_000) {
    return `Rp ${(value / 1_000_000).toFixed(1)}M`;
  }
  if (value >= 1_000) {
    return `Rp ${(value / 1_000).toFixed(0)}K`;
  }
  return `Rp ${value.toFixed(0)}`;
}

export function TaxSummaryCard({ data, loading, year }: TaxSummaryCardProps) {
  const [downloading, setDownloading] = useState<string | null>(null);

  const handleDownload = async (format: "pdf" | "excel") => {
    setDownloading(format);
    try {
      await downloadFile(
        `/analytics/report?format=${format}&year=${year}`,
        `financial-report-${year}.${format === "pdf" ? "pdf" : "xlsx"}`
      );
    } catch (err) {
      console.error(`Failed to download ${format} report:`, err);
    } finally {
      setDownloading(null);
    }
  };

  const totalTax = data.reduce((sum, d) => sum + d.tax, 0);
  const totalRevenue = data.reduce((sum, d) => sum + d.revenue, 0);

  return (
    <div className="rounded-xl border border-gray-100 bg-white p-6 shadow-sm space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="text-lg font-semibold text-gray-700">
          Tax Summary {year}
        </h3>
        <div className="flex items-center gap-2">
          <button
            onClick={() => handleDownload("pdf")}
            disabled={downloading !== null}
            className="rounded-lg border border-gray-300 px-3 py-1.5 text-xs font-medium text-gray-600 hover:bg-gray-50 disabled:opacity-50 transition-colors"
          >
            {downloading === "pdf" ? "Generating..." : "PDF Report"}
          </button>
          <button
            onClick={() => handleDownload("excel")}
            disabled={downloading !== null}
            className="rounded-lg bg-green-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-green-700 disabled:opacity-50 transition-colors shadow-sm"
          >
            {downloading === "excel" ? "Generating..." : "Export Excel"}
          </button>
        </div>
      </div>

      {loading ? (
        <div className="space-y-3">
          {[1, 2, 3].map((i) => (
            <div key={i} className="h-5 bg-gray-200 rounded animate-pulse" />
          ))}
        </div>
      ) : data.length === 0 ? (
        <p className="text-gray-400 text-sm py-4">No tax data for {year}</p>
      ) : (
        <div className="overflow-hidden rounded-lg border border-gray-100">
          <table className="w-full text-sm">
            <thead className="bg-gray-50 text-xs uppercase text-gray-600 tracking-wide">
              <tr>
                <th className="px-4 py-2.5 text-left font-medium">Month</th>
                <th className="px-4 py-2.5 text-right font-medium">Revenue</th>
                <th className="px-4 py-2.5 text-right font-medium">Tax</th>
              </tr>
            </thead>
            <tbody>
              {data.map((d) => (
                <tr key={d.label} className="border-t border-gray-100 hover:bg-gray-50 transition-colors">
                  <td className="px-4 py-2.5 font-medium text-gray-700">{d.label}</td>
                  <td className="px-4 py-2.5 text-right font-mono text-gray-600">
                    {formatIDR(d.revenue)}
                  </td>
                  <td className="px-4 py-2.5 text-right font-mono text-amber-600 font-medium">
                    {formatIDR(d.tax)}
                  </td>
                </tr>
              ))}
            </tbody>
            <tfoot>
              <tr className="border-t-2 border-gray-200 bg-gray-50 font-semibold">
                <td className="px-4 py-2.5 text-gray-700">Total</td>
                <td className="px-4 py-2.5 text-right font-mono text-gray-800">
                  {formatIDR(totalRevenue)}
                </td>
                <td className="px-4 py-2.5 text-right font-mono text-amber-600">
                  {formatIDR(totalTax)}
                </td>
              </tr>
            </tfoot>
          </table>
        </div>
      )}
    </div>
  );
}
