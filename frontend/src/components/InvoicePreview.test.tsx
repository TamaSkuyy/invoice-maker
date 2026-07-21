import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import InvoicePreview from "./InvoicePreview";
import type { Invoice } from "../types/invoice";

const mockInvoice: Invoice = {
  id: "abc12345-6789",
  client_name: "PT Maju Jaya",
  date: "2026-07-21",
  due_date: "2026-08-21",
  tax_rate: 11,
  total_amount: 555000,
  items: [
    { description: "Website Development", qty: 1, price: 500000 },
    { description: "Hosting", qty: 1, price: 50000 },
  ],
};

describe("InvoicePreview", () => {
  it("should render invoice header", () => {
    render(<InvoicePreview invoice={mockInvoice} />);

    expect(screen.getByText("INVOICE")).toBeInTheDocument();
    expect(screen.getByText("#abc12345-6789")).toBeInTheDocument();
    expect(screen.getByText("PT Maju Jaya")).toBeInTheDocument();
  });

  it("should render line items", () => {
    render(<InvoicePreview invoice={mockInvoice} />);

    expect(screen.getByText("Website Development")).toBeInTheDocument();
    expect(screen.getByText("Hosting")).toBeInTheDocument();
  });

  it("should render tax rate", () => {
    render(<InvoicePreview invoice={mockInvoice} />);

    expect(screen.getByText(/11%/)).toBeInTheDocument();
  });

  it("should render total amount", () => {
    render(<InvoicePreview invoice={mockInvoice} />);

    expect(screen.getByText(/555/)).toBeInTheDocument();
  });
});
