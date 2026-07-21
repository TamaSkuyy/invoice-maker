import {
  PieChart,
  Pie,
  Cell,
  Tooltip,
  ResponsiveContainer,
  Legend,
} from "recharts";
import type { TopClientData } from "../types/analytics";

interface TopClientsChartProps {
  data: TopClientData[];
  loading: boolean;
}

const COLORS = ["#2563eb", "#7c3aed", "#059669", "#d97706", "#dc2626"];

function formatTooltip(value: number): string {
  return `Rp ${value.toLocaleString("id-ID")}`;
}

function truncateName(name: string, maxLen: number = 18): string {
  return name.length > maxLen ? name.slice(0, maxLen - 2) + "…" : name;
}

export function TopClientsChart({ data, loading }: TopClientsChartProps) {
  return (
    <div className="rounded-xl border border-gray-200 bg-white p-5 shadow-sm">
      <h3 className="text-sm font-semibold text-gray-700 uppercase tracking-wide mb-4">
        Top Clients
      </h3>

      {loading ? (
        <div className="h-64 flex items-center justify-center">
          <div className="animate-spin h-6 w-6 border-2 border-blue-500 border-t-transparent rounded-full" />
        </div>
      ) : data.length === 0 ? (
        <div className="h-64 flex items-center justify-center text-gray-400 text-sm">
          No client data yet
        </div>
      ) : (
        <ResponsiveContainer width="100%" height={260}>
          <PieChart>
            <Pie
              data={data}
              dataKey="total"
              nameKey="client_name"
              cx="50%"
              cy="50%"
              outerRadius={90}
              innerRadius={45}
              paddingAngle={3}
              label={({ client_name }: { client_name: string }) => truncateName(client_name)}
              labelLine={{ stroke: "#9ca3af", strokeWidth: 1 }}
            >
              {data.map((_, i) => (
                <Cell key={i} fill={COLORS[i % COLORS.length]} />
              ))}
            </Pie>
            <Tooltip formatter={(value: number) => [formatTooltip(Number(value)), "Revenue"]} />
            <Legend
              formatter={(value: string) => truncateName(value, 14)}
              wrapperStyle={{ fontSize: 12 }}
            />
          </PieChart>
        </ResponsiveContainer>
      )}
    </div>
  );
}
