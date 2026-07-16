import { useState, useEffect, useRef } from "react";
import { apiFetch } from "../utils/api";
import type { Product } from "../types/invoice";

interface Props {
  onPick: (description: string, price: number) => void;
}

export default function ProductSelector({ onPick }: Props) {
  const [products, setProducts] = useState<Product[]>([]);
  const [open, setOpen] = useState(false);
  const [loading, setLoading] = useState(true);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    apiFetch<Product[]>("/products")
      .then((data) => setProducts(data || []))
      .catch((err) => console.error("Failed to fetch products:", err))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    const handleClick = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, []);

  const handlePick = (product: Product) => {
    onPick(product.description || product.name, product.default_price);
    setOpen(false);
  };

  if (loading || products.length === 0) return null;

  return (
    <div ref={ref} className="relative inline-block">
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className="text-xs text-blue-500 hover:text-blue-700 hover:underline"
        title="Pick a saved product"
      >
        +Pick
      </button>

      {open && (
        <div className="absolute left-0 top-6 z-10 w-64 rounded-lg border border-gray-200 bg-white shadow-lg py-1 max-h-48 overflow-y-auto">
          {products.map((p) => (
            <button
              key={p.id}
              type="button"
              onClick={() => handlePick(p)}
              className="w-full text-left px-4 py-2 text-sm hover:bg-blue-50 flex justify-between"
            >
              <span className="text-gray-800 truncate mr-2">
                {p.name}
              </span>
              <span className="text-gray-400 font-mono text-xs whitespace-nowrap">
                Rp {p.default_price.toFixed(2)}
              </span>
            </button>
          ))}
          {products.length === 0 && (
            <p className="px-4 py-2 text-sm text-gray-400">
              No products saved yet.
            </p>
          )}
        </div>
      )}
    </div>
  );
}
