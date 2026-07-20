import type { AnalyticsOverview } from "../types/analytics";

interface DashboardCardsProps {
  data: AnalyticsOverview | null;
  loading: boolean;
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

function SkeletonCard() {
  return (
    <div className="rounded-xl border border-gray-200 bg-white p-5 animate-pulse">
      <div className="h-3 w-20 bg-gray-200 rounded mb-3" />
      <div className="h-7 w-32 bg-gray-200 rounded" />
    </div>
  );
}

export function DashboardCards({ data, loading }: DashboardCardsProps) {
  if (loading) {
    return (
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
        <SkeletonCard />
        <SkeletonCard />
        <SkeletonCard />
        <SkeletonCard />
      </div>
    );
  }

  if (!data) return null;

  const cards = [
    { label: "Total Revenue", value: formatIDR(data.total_revenue), color: "text-blue-600" },
    { label: "Total Invoices", value: `${data.total_invoices}`, color: "text-emerald-600" },
    { label: "Total Clients", value: `${data.total_clients}`, color: "text-violet-600" },
    { label: "Avg Invoice", value: formatIDR(data.avg_invoice_value), color: "text-amber-600" },
    { label: "Paid Revenue", value: formatIDR(data.paid_amount), color: "text-green-600" },
    { label: "Pending Revenue", value: formatIDR(data.pending_amount), color: "text-amber-600" },
    { label: "Overdue Invoices", value: `${data.overdue_count}`, color: "text-red-600" },
  ];

  return (
    <div className="grid grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
      {cards.map((card) => (
        <div
          key={card.label}
          className="rounded-xl border border-gray-200 bg-white p-5 shadow-sm hover:shadow-md transition-shadow"
        >
          <p className="text-xs font-medium uppercase tracking-wide text-gray-500 mb-1">
            {card.label}
          </p>
          <p className={`text-2xl font-bold ${card.color}`}>{card.value}</p>
        </div>
      ))}
    </div>
  );
}
