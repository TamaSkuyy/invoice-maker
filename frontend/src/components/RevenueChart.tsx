import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from "recharts";
import type { RevenueDataPoint } from "../types/analytics";

interface RevenueChartProps {
  data: RevenueDataPoint[];
  loading: boolean;
  year: number;
  onYearChange: (year: number) => void;
}

function formatTick(value: number): string {
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(0)}M`;
  if (value >= 1_000) return `${(value / 1_000).toFixed(0)}K`;
  return `${value}`;
}

function formatTooltip(value: number): string {
  return `Rp ${value.toLocaleString("id-ID")}`;
}

const currentYear = new Date().getFullYear();
const years = Array.from({ length: 5 }, (_, i) => currentYear - i);

export function RevenueChart({ data, loading, year, onYearChange }: RevenueChartProps) {
  return (
    <div className="rounded-xl border border-gray-200 bg-white p-5 shadow-sm">
      <div className="flex items-center justify-between mb-4">
        <h3 className="text-sm font-semibold text-gray-700 uppercase tracking-wide">
          Revenue Overview
        </h3>
        <select
          value={year}
          onChange={(e) => onYearChange(Number(e.target.value))}
          className="rounded-lg border border-gray-300 px-3 py-1.5 text-sm text-gray-600 focus:outline-none focus:ring-2 focus:ring-blue-500"
        >
          {years.map((y) => (
            <option key={y} value={y}>
              {y}
            </option>
          ))}
        </select>
      </div>

      {loading ? (
        <div className="h-64 flex items-center justify-center">
          <div className="animate-spin h-6 w-6 border-2 border-blue-500 border-t-transparent rounded-full" />
        </div>
      ) : data.length === 0 ? (
        <div className="h-64 flex items-center justify-center text-gray-400 text-sm">
          No invoices for {year}
        </div>
      ) : (
        <ResponsiveContainer width="100%" height={260}>
          <BarChart data={data} margin={{ top: 5, right: 10, left: 10, bottom: 5 }}>
            <CartesianGrid strokeDasharray="3 3" stroke="#f1f5f9" />
            <XAxis dataKey="label" tick={{ fontSize: 12, fill: "#6b7280" }} />
            <YAxis tickFormatter={formatTick} tick={{ fontSize: 12, fill: "#6b7280" }} />
            <Tooltip formatter={(value: number) => [formatTooltip(Number(value)), "Revenue"]} />
            <Bar dataKey="total" fill="#2563eb" radius={[4, 4, 0, 0]} maxBarSize={40} />
          </BarChart>
        </ResponsiveContainer>
      )}
    </div>
  );
}
