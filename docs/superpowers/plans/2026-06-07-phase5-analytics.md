# Phase 5: Reporting & Analytics — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add analytics dashboard with revenue charts and downloadable financial reports.

**Architecture:** New `/api/analytics` endpoints aggregate invoice data via SQL (`SUM`, `GROUP BY`, `date_trunc`), returning pre-computed analytics. Frontend renders dashboard cards + Recharts charts above the existing invoice form. Report generation reuses fpdf/excelize patterns from Phase 3.

**Tech Stack:** Go 1.21+, Gin, PostgreSQL (pgx), Recharts (React), TypeScript, fpdf, excelize

**Spec:** `docs/superpowers/specs/2026-06-07-phase5-analytics-design.md`

---

## File Map

```
backend/
  analytics.go              [NEW] — Types, SQL query helpers, 5 handler functions, PDF/Excel report generation
  main.go                   [MODIFY: L1045-L1048] — Register /api/analytics route group

frontend/
  src/types/analytics.ts    [NEW] — TypeScript interfaces for analytics API responses
  src/components/
    DashboardCards.tsx      [NEW] — 4 metric cards (revenue, invoices, clients, avg)
    RevenueChart.tsx        [NEW] — Recharts BarChart monthly revenue
    TopClientsChart.tsx     [NEW] — Recharts PieChart top 5 clients
    TaxSummaryCard.tsx      [NEW] — Tax table + download report buttons
    ProtectedInvoiceDashboard.tsx  [MODIFY] — Add dashboard section above form
```

---

### Task 1: Create Backend Analytics Handlers

**Files:**
- Create: `backend/analytics.go`

This file contains all analytics types, SQL query helpers, handler functions, and report generation. Putting it in a separate file keeps `main.go` (already 1048 lines) from growing further.

- [ ] **Step 1: Write `backend/analytics.go`**

```go
package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-pdf/fpdf"
	"github.com/xuri/excelize/v2"
)

// ── Analytics Types ─────────────────────────────────────────────────────────

type AnalyticsOverview struct {
	TotalRevenue   float64 `json:"total_revenue"`
	TotalInvoices  int     `json:"total_invoices"`
	TotalClients   int     `json:"total_clients"`
	AvgInvoiceValue float64 `json:"avg_invoice_value"`
}

type RevenueDataPoint struct {
	Label string  `json:"label"`
	Total float64 `json:"total"`
	Count int     `json:"count"`
}

type RevenueResponse struct {
	Period string             `json:"period"`
	Data   []RevenueDataPoint `json:"data"`
}

type TopClientData struct {
	ClientName string  `json:"client_name"`
	Total      float64 `json:"total"`
	Count      int     `json:"count"`
}

type TopClientsResponse struct {
	Clients []TopClientData `json:"clients"`
}

type TaxDataPoint struct {
	Label   string  `json:"label"`
	Tax     float64 `json:"tax"`
	Revenue float64 `json:"revenue"`
}

type TaxSummaryResponse struct {
	Data []TaxDataPoint `json:"data"`
}

// ── Helper: parse year query param ──────────────────────────────────────────

func parseYear(c *gin.Context) int {
	yearStr := c.DefaultQuery("year", strconv.Itoa(time.Now().Year()))
	year, err := strconv.Atoi(yearStr)
	if err != nil || year < 2000 || year > 2100 {
		year = time.Now().Year()
	}
	return year
}

// ── Handler: GET /api/analytics/overview ────────────────────────────────────

func handleAnalyticsOverview(c *gin.Context) {
	userID, _ := c.Get("user_id")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	var o AnalyticsOverview
	err := db.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(total_amount), 0),
			COUNT(*),
			COUNT(DISTINCT client_id),
			CASE WHEN COUNT(*) > 0 THEN SUM(total_amount) / COUNT(*) ELSE 0 END
		FROM invoices
		WHERE user_id = $1
	`, userID).Scan(&o.TotalRevenue, &o.TotalInvoices, &o.TotalClients, &o.AvgInvoiceValue)
	if err != nil {
		log.Printf("analytics overview error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch analytics"})
		return
	}

	o.AvgInvoiceValue = round2(o.AvgInvoiceValue)
	c.JSON(http.StatusOK, o)
}

// ── Handler: GET /api/analytics/revenue?year= ───────────────────────────────

func handleAnalyticsRevenue(c *gin.Context) {
	userID, _ := c.Get("user_id")
	year := parseYear(c)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	rows, err := db.Query(ctx, `
		SELECT
			TO_CHAR(date, 'Mon') as label,
			EXTRACT(MONTH FROM date) as month,
			COALESCE(SUM(total_amount), 0) as total,
			COUNT(*) as count
		FROM invoices
		WHERE user_id = $1 AND EXTRACT(YEAR FROM date) = $2
		GROUP BY month, label
		ORDER BY month
	`, userID, year)
	if err != nil {
		log.Printf("analytics revenue error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch revenue data"})
		return
	}
	defer rows.Close()

	var data []RevenueDataPoint
	for rows.Next() {
		var d RevenueDataPoint
		if err := rows.Scan(&d.Label, new(float64), &d.Total, &d.Count); err != nil {
			log.Printf("scan revenue error: %v", err)
			continue
		}
		d.Total = round2(d.Total)
		data = append(data, d)
	}
	if data == nil {
		data = []RevenueDataPoint{}
	}

	c.JSON(http.StatusOK, RevenueResponse{Period: "monthly", Data: data})
}

// ── Handler: GET /api/analytics/top-clients?limit= ──────────────────────────

func handleAnalyticsTopClients(c *gin.Context) {
	userID, _ := c.Get("user_id")
	limitStr := c.DefaultQuery("limit", "5")
	limit, _ := strconv.Atoi(limitStr)
	if limit < 1 || limit > 50 {
		limit = 5
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	rows, err := db.Query(ctx, `
		SELECT
			COALESCE(c.name, i.client_name) as client_name,
			COALESCE(SUM(i.total_amount), 0) as total,
			COUNT(*) as count
		FROM invoices i
		LEFT JOIN clients c ON i.client_id = c.id
		WHERE i.user_id = $1
		GROUP BY client_name
		ORDER BY total DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		log.Printf("analytics top-clients error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch top clients"})
		return
	}
	defer rows.Close()

	var clients []TopClientData
	for rows.Next() {
		var d TopClientData
		if err := rows.Scan(&d.ClientName, &d.Total, &d.Count); err != nil {
			log.Printf("scan top-clients error: %v", err)
			continue
		}
		d.Total = round2(d.Total)
		clients = append(clients, d)
	}
	if clients == nil {
		clients = []TopClientData{}
	}

	c.JSON(http.StatusOK, TopClientsResponse{Clients: clients})
}

// ── Handler: GET /api/analytics/tax-summary?year= ───────────────────────────

func handleAnalyticsTaxSummary(c *gin.Context) {
	userID, _ := c.Get("user_id")
	year := parseYear(c)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	rows, err := db.Query(ctx, `
		SELECT
			TO_CHAR(date, 'Mon') as label,
			EXTRACT(MONTH FROM date) as month,
			COALESCE(SUM(total_amount - total_amount / (1 + tax_rate / 100)), 0) as tax,
			COALESCE(SUM(total_amount), 0) as revenue
		FROM invoices
		WHERE user_id = $1 AND EXTRACT(YEAR FROM date) = $2
		GROUP BY month, label
		ORDER BY month
	`, userID, year)
	if err != nil {
		log.Printf("analytics tax-summary error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch tax summary"})
		return
	}
	defer rows.Close()

	var data []TaxDataPoint
	for rows.Next() {
		var d TaxDataPoint
		if err := rows.Scan(&d.Label, new(float64), &d.Tax, &d.Revenue); err != nil {
			log.Printf("scan tax-summary error: %v", err)
			continue
		}
		d.Tax = round2(d.Tax)
		d.Revenue = round2(d.Revenue)
		data = append(data, d)
	}
	if data == nil {
		data = []TaxDataPoint{}
	}

	c.JSON(http.StatusOK, TaxSummaryResponse{Data: data})
}

// ── Handler: GET /api/analytics/report?format=pdf|excel&year= ───────────────

func handleAnalyticsReport(c *gin.Context) {
	userID, _ := c.Get("user_id")
	format := c.DefaultQuery("format", "pdf")
	year := parseYear(c)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	// Gather report data
	var overview AnalyticsOverview
	db.QueryRow(ctx, `
		SELECT COALESCE(SUM(total_amount),0), COUNT(*),
			COUNT(DISTINCT client_id),
			CASE WHEN COUNT(*)>0 THEN SUM(total_amount)/COUNT(*) ELSE 0 END
		FROM invoices WHERE user_id=$1
	`, userID).Scan(&overview.TotalRevenue, &overview.TotalInvoices, &overview.TotalClients, &overview.AvgInvoiceValue)
	overview.AvgInvoiceValue = round2(overview.AvgInvoiceValue)

	revRows, _ := db.Query(ctx, `
		SELECT TO_CHAR(date,'Mon'), EXTRACT(MONTH FROM date),
			COALESCE(SUM(total_amount),0), COUNT(*)
		FROM invoices WHERE user_id=$1 AND EXTRACT(YEAR FROM date)=$2
		GROUP BY month, label ORDER BY month
	`, userID, year)

	var revenue []RevenueDataPoint
	for revRows.Next() {
		var d RevenueDataPoint
		revRows.Scan(&d.Label, new(float64), &d.Total, &d.Count)
		d.Total = round2(d.Total)
		revenue = append(revenue, d)
	}
	revRows.Close()

	taxRows, _ := db.Query(ctx, `
		SELECT TO_CHAR(date,'Mon'), EXTRACT(MONTH FROM date),
			COALESCE(SUM(total_amount - total_amount/(1+tax_rate/100)),0),
			COALESCE(SUM(total_amount),0)
		FROM invoices WHERE user_id=$1 AND EXTRACT(YEAR FROM date)=$2
		GROUP BY month, label ORDER BY month
	`, userID, year)

	var taxData []TaxDataPoint
	for taxRows.Next() {
		var d TaxDataPoint
		taxRows.Scan(&d.Label, new(float64), &d.Tax, &d.Revenue)
		d.Tax = round2(d.Tax)
		d.Revenue = round2(d.Revenue)
		taxData = append(taxData, d)
	}
	taxRows.Close()

	switch format {
	case "excel":
		data, err := generateAnalyticsExcel(overview, revenue, taxData, year)
		if err != nil {
			log.Printf("analytics excel error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate report"})
			return
		}
		c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=financial-report-%d.xlsx", year))
		c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", data)
	default:
		data, err := generateAnalyticsPDF(overview, revenue, taxData, year)
		if err != nil {
			log.Printf("analytics pdf error: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate report"})
			return
		}
		c.Header("Content-Type", "application/pdf")
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=financial-report-%d.pdf", year))
		c.Data(http.StatusOK, "application/pdf", data)
	}
}

// ── PDF Report Generation ───────────────────────────────────────────────────

func generateAnalyticsPDF(overview AnalyticsOverview, revenue []RevenueDataPoint, taxData []TaxDataPoint, year int) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetAutoPageBreak(true, 20)
	pdf.SetMargins(15, 15, 15)
	pdf.AddPage()

	pageW, _ := pdf.GetPageSize()
	usableW := pageW - 30

	headerColor := []int{37, 99, 235}
	darkColor := []int{31, 41, 55}
	grayColor := []int{107, 114, 128}

	// Title
	pdf.SetFillColor(headerColor[0], headerColor[1], headerColor[2])
	pdf.Rect(0, 0, pageW, 36, "F")
	pdf.SetFont("Helvetica", "B", 22)
	pdf.SetTextColor(255, 255, 255)
	pdf.SetY(10)
	pdf.CellFormat(0, 12, fmt.Sprintf("Financial Report — %d", year), "", 1, "C", false, 0, "")
	pdf.Ln(16)

	// Overview cards
	pdf.SetFont("Helvetica", "B", 12)
	pdf.SetTextColor(darkColor[0], darkColor[1], darkColor[2])
	pdf.Cell(0, 8, "Overview")
	pdf.Ln(10)

	cardW := usableW/2 - 2
	cardH := 16.0

	drawCard := func(label, value string, x, y float64) {
		pdf.SetFillColor(249, 250, 251)
		pdf.RoundedRect(x, y, cardW, cardH, 2, "1234", "F")
		pdf.SetFont("Helvetica", "", 9)
		pdf.SetTextColor(grayColor[0], grayColor[1], grayColor[2])
		pdf.SetXY(x+4, y+2)
		pdf.CellFormat(cardW-8, 5, label, "", 0, "L", false, 0, "")
		pdf.SetFont("Helvetica", "B", 13)
		pdf.SetTextColor(37, 99, 235)
		pdf.SetXY(x+4, y+8)
		pdf.CellFormat(cardW-8, 6, value, "", 0, "L", false, 0, "")
	}

	yCards := pdf.GetY()
	col1X := 15.0
	col2X := 15.0 + cardW + 4
	drawCard("Total Revenue", formatMoney(overview.TotalRevenue), col1X, yCards)
	drawCard("Total Invoices", fmt.Sprintf("%d invoices", overview.TotalInvoices), col2X, yCards)
	drawCard("Total Clients", fmt.Sprintf("%d clients", overview.TotalClients), col1X, yCards+cardH+4)
	drawCard("Avg Invoice Value", formatMoney(overview.AvgInvoiceValue), col2X, yCards+cardH+4)
	pdf.Ln(cardH*2 + 14)

	// Monthly breakdown table
	pdf.SetFont("Helvetica", "B", 12)
	pdf.SetTextColor(darkColor[0], darkColor[1], darkColor[2])
	pdf.Cell(0, 8, "Monthly Breakdown")
	pdf.Ln(10)

	// Table header
	colMonth := 30.0
	colRevenue := (usableW - colMonth) / 2
	colTax := colRevenue

	pdf.SetFillColor(37, 99, 235)
	pdf.SetTextColor(255, 255, 255)
	pdf.SetFont("Helvetica", "B", 9)
	y := pdf.GetY()
	pdf.Rect(15, y, usableW, 7, "F")
	pdf.SetXY(15, y)
	pdf.CellFormat(colMonth, 7, "  Month", "", 0, "L", false, 0, "")
	pdf.CellFormat(colRevenue, 7, "Revenue", "", 0, "R", false, 0, "")
	pdf.CellFormat(colTax, 7, "Tax", "", 0, "R", false, 0, "")
	pdf.Ln(7)

	for i, r := range revenue {
		taxVal := 0.0
		if i < len(taxData) {
			taxVal = taxData[i].Tax
		}
		bg := []int{255, 255, 255}
		if i%2 == 0 {
			bg = []int{249, 250, 251}
		}
		pdf.SetFillColor(bg[0], bg[1], bg[2])
		y := pdf.GetY()
		pdf.Rect(15, y, usableW, 6.5, "F")
		pdf.SetTextColor(darkColor[0], darkColor[1], darkColor[2])
		pdf.SetFont("Helvetica", "", 10)
		pdf.SetXY(15, y+1)
		pdf.CellFormat(colMonth, 5, "  "+r.Label, "", 0, "L", false, 0, "")
		pdf.CellFormat(colRevenue, 5, formatMoney(r.Total), "", 0, "R", false, 0, "")
		pdf.CellFormat(colTax, 5, formatMoney(taxVal), "", 0, "R", false, 0, "")
		pdf.Ln(6.5)
	}

	// Footer
	pdf.SetY(-20)
	pdf.SetFont("Helvetica", "I", 8)
	pdf.SetTextColor(180, 180, 180)
	pdf.CellFormat(0, 4, fmt.Sprintf("Generated by Invoice Maker — %s", time.Now().Format("2006-01-02 15:04")), "", 0, "C", false, 0, "")

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("failed to generate report PDF: %w", err)
	}
	return buf.Bytes(), nil
}

// ── Excel Report Generation ─────────────────────────────────────────────────

func generateAnalyticsExcel(overview AnalyticsOverview, revenue []RevenueDataPoint, taxData []TaxDataPoint, year int) ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()

	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 11, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"2563EB"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	moneyStyle, _ := f.NewStyle(&excelize.Style{
		NumFmt:     36,
		Alignment:  &excelize.Alignment{Horizontal: "right"},
	})
	centerStyle, _ := f.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})

	// Sheet 1: Overview
	overviewSheet := "Overview"
	f.SetSheetName("Sheet1", overviewSheet)
	f.SetCellValue(overviewSheet, "A1", fmt.Sprintf("Financial Report — %d", year))
	f.SetCellValue(overviewSheet, "A3", "Total Revenue")
	f.SetCellValue(overviewSheet, "B3", overview.TotalRevenue)
	f.SetCellValue(overviewSheet, "A4", "Total Invoices")
	f.SetCellValue(overviewSheet, "B4", overview.TotalInvoices)
	f.SetCellValue(overviewSheet, "A5", "Total Clients")
	f.SetCellValue(overviewSheet, "B5", overview.TotalClients)
	f.SetCellValue(overviewSheet, "A6", "Avg Invoice Value")
	f.SetCellValue(overviewSheet, "B6", overview.AvgInvoiceValue)
	f.SetColWidth(overviewSheet, "A", "A", 22)
	f.SetColWidth(overviewSheet, "B", "B", 18)
	f.SetCellStyle(overviewSheet, "B3", "B6", moneyStyle)

	// Sheet 2: Monthly
	monthlySheet := "Monthly"
	f.NewSheet(monthlySheet)
	f.SetCellValue(monthlySheet, "A1", "Month")
	f.SetCellValue(monthlySheet, "B1", "Revenue")
	f.SetCellValue(monthlySheet, "C1", "Invoice Count")
	f.SetCellStyle(monthlySheet, "A1", "C1", headerStyle)
	for i, r := range revenue {
		row := i + 2
		f.SetCellValue(monthlySheet, cellName(1, row), r.Label)
		f.SetCellValue(monthlySheet, cellName(2, row), r.Total)
		f.SetCellValue(monthlySheet, cellName(3, row), r.Count)
		f.SetCellStyle(monthlySheet, cellName(2, row), cellName(2, row), moneyStyle)
		f.SetCellStyle(monthlySheet, cellName(3, row), cellName(3, row), centerStyle)
	}
	f.SetColWidth(monthlySheet, "A", "A", 12)
	f.SetColWidth(monthlySheet, "B", "B", 18)
	f.SetColWidth(monthlySheet, "C", "C", 16)

	// Sheet 3: Tax Summary
	taxSheet := "Tax Summary"
	f.NewSheet(taxSheet)
	f.SetCellValue(taxSheet, "A1", "Month")
	f.SetCellValue(taxSheet, "B1", "Revenue")
	f.SetCellValue(taxSheet, "C1", "Tax")
	f.SetCellStyle(taxSheet, "A1", "C1", headerStyle)
	for i, t := range taxData {
		row := i + 2
		f.SetCellValue(taxSheet, cellName(1, row), t.Label)
		f.SetCellValue(taxSheet, cellName(2, row), t.Revenue)
		f.SetCellValue(taxSheet, cellName(3, row), t.Tax)
		f.SetCellStyle(taxSheet, cellName(2, row), cellName(3, row), moneyStyle)
	}
	f.SetColWidth(taxSheet, "A", "A", 12)
	f.SetColWidth(taxSheet, "B", "C", 18)

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("failed to generate report Excel: %w", err)
	}
	return buf.Bytes(), nil
}
```

- [ ] **Step 2: Verify backend compiles**

Run: `cd backend && go build ./...`
Expected: exit 0, no errors

- [ ] **Step 3: Commit backend**

```bash
git add backend/analytics.go
git commit -m "feat: add analytics handlers with overview, revenue, top-clients, tax-summary, and report endpoints"
```

---

### Task 2: Register Analytics Routes in main.go

**Files:**
- Modify: `backend/main.go` — add route group after products group (around L1045)

- [ ] **Step 1: Add analytics route group**

After the products group closing `}` (around L1045, after `r.Run(":8080")`'s line), insert the analytics route group BEFORE `r.Run(":8080")`:

In `backend/main.go`, find:
```go
			}
		}

		r.Run(":8080")
	}
```

Replace with:
```go
			}
		}

		// Analytics routes (protected)
		analytics := r.Group("/api/analytics")
		analytics.Use(authenticate())
		{
			analytics.GET("/overview", handleAnalyticsOverview)
			analytics.GET("/revenue", handleAnalyticsRevenue)
			analytics.GET("/top-clients", handleAnalyticsTopClients)
			analytics.GET("/tax-summary", handleAnalyticsTaxSummary)
			analytics.GET("/report", handleAnalyticsReport)
		}

		r.Run(":8080")
	}
```

- [ ] **Step 2: Verify backend compiles and routes are registered**

Run: `cd backend && go build ./...`
Expected: exit 0, no errors (handler functions referenced from analytics.go resolve correctly)

- [ ] **Step 3: Commit**

```bash
git add backend/main.go
git commit -m "feat: register /api/analytics routes with JWT protection"
```

---

### Task 3: Create TypeScript Types for Analytics

**Files:**
- Create: `frontend/src/types/analytics.ts`

- [ ] **Step 1: Write `frontend/src/types/analytics.ts`**

```typescript
export interface AnalyticsOverview {
  total_revenue: number
  total_invoices: number
  total_clients: number
  avg_invoice_value: number
}

export interface RevenueDataPoint {
  label: string
  total: number
  count: number
}

export interface RevenueResponse {
  period: "monthly"
  data: RevenueDataPoint[]
}

export interface TopClientData {
  client_name: string
  total: number
  count: number
}

export interface TopClientsResponse {
  clients: TopClientData[]
}

export interface TaxDataPoint {
  label: string
  tax: number
  revenue: number
}

export interface TaxSummaryResponse {
  data: TaxDataPoint[]
}
```

- [ ] **Step 2: Verify TypeScript compiles**

Run: `cd frontend && npx tsc --noEmit`
Expected: exit 0, no new errors (file is self-contained types)

- [ ] **Step 3: Commit**

```bash
git add frontend/src/types/analytics.ts
git commit -m "feat: add analytics TypeScript interfaces"
```

---

### Task 4: Create DashboardCards Component

**Files:**
- Create: `frontend/src/components/DashboardCards.tsx`

- [ ] **Step 1: Write `frontend/src/components/DashboardCards.tsx`**

```tsx
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
```

- [ ] **Step 2: Verify TypeScript**

Run: `cd frontend && npx tsc --noEmit`
Expected: exit 0

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/DashboardCards.tsx
git commit -m "feat: add DashboardCards component with 4 metric cards"
```

---

### Task 5: Create RevenueChart Component

**Files:**
- Create: `frontend/src/components/RevenueChart.tsx`

- [ ] **Step 1: Install Recharts**

Run: `cd frontend && npm install recharts`

- [ ] **Step 2: Write `frontend/src/components/RevenueChart.tsx`**

```tsx
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
            <Tooltip formatter={(value: number) => [formatTooltip(value), "Revenue"]} />
            <Bar dataKey="total" fill="#2563eb" radius={[4, 4, 0, 0]} maxBarSize={40} />
          </BarChart>
        </ResponsiveContainer>
      )}
    </div>
  );
}
```

- [ ] **Step 3: Verify TypeScript**

Run: `cd frontend && npx tsc --noEmit`
Expected: exit 0

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/RevenueChart.tsx frontend/package.json frontend/package-lock.json
git commit -m "feat: add RevenueChart with Recharts bar chart and year selector"
```

---

### Task 6: Create TopClientsChart Component

**Files:**
- Create: `frontend/src/components/TopClientsChart.tsx`

- [ ] **Step 1: Write `frontend/src/components/TopClientsChart.tsx`**

```tsx
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
              label={({ client_name }) => truncateName(client_name)}
              labelLine={{ stroke: "#9ca3af", strokeWidth: 1 }}
            >
              {data.map((_, i) => (
                <Cell key={i} fill={COLORS[i % COLORS.length]} />
              ))}
            </Pie>
            <Tooltip formatter={(value: number) => [formatTooltip(value), "Revenue"]} />
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
```

- [ ] **Step 2: Verify TypeScript**

Run: `cd frontend && npx tsc --noEmit`
Expected: exit 0

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/TopClientsChart.tsx
git commit -m "feat: add TopClientsChart with Recharts pie chart"
```

---

### Task 7: Create TaxSummaryCard Component

**Files:**
- Create: `frontend/src/components/TaxSummaryCard.tsx`

- [ ] **Step 1: Write `frontend/src/components/TaxSummaryCard.tsx`**

```tsx
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
    <div className="rounded-xl border border-gray-200 bg-white p-5 shadow-sm">
      <div className="flex items-center justify-between mb-4">
        <h3 className="text-sm font-semibold text-gray-700 uppercase tracking-wide">
          Tax Summary {year}
        </h3>
        <div className="flex items-center gap-2">
          <button
            onClick={() => handleDownload("pdf")}
            disabled={downloading !== null}
            className="rounded-lg border border-red-300 px-3 py-1.5 text-xs font-medium text-red-600 hover:bg-red-50 disabled:opacity-50 transition-colors"
          >
            {downloading === "pdf" ? "Generating..." : "PDF"}
          </button>
          <button
            onClick={() => handleDownload("excel")}
            disabled={downloading !== null}
            className="rounded-lg border border-green-300 px-3 py-1.5 text-xs font-medium text-green-600 hover:bg-green-50 disabled:opacity-50 transition-colors"
          >
            {downloading === "excel" ? "Generating..." : "Excel"}
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
        <>
          <div className="overflow-hidden rounded-lg border border-gray-100">
            <table className="w-full text-sm">
              <thead className="bg-gray-50 text-xs uppercase text-gray-500">
                <tr>
                  <th className="px-3 py-2 text-left">Month</th>
                  <th className="px-3 py-2 text-right">Revenue</th>
                  <th className="px-3 py-2 text-right">Tax</th>
                </tr>
              </thead>
              <tbody>
                {data.map((d) => (
                  <tr key={d.label} className="border-t border-gray-100">
                    <td className="px-3 py-2 font-medium text-gray-700">{d.label}</td>
                    <td className="px-3 py-2 text-right font-mono text-gray-600">
                      {formatIDR(d.revenue)}
                    </td>
                    <td className="px-3 py-2 text-right font-mono text-amber-600 font-medium">
                      {formatIDR(d.tax)}
                    </td>
                  </tr>
                ))}
              </tbody>
              <tfoot>
                <tr className="border-t-2 border-gray-200 bg-gray-50 font-semibold">
                  <td className="px-3 py-2 text-gray-700">Total</td>
                  <td className="px-3 py-2 text-right font-mono text-gray-800">
                    {formatIDR(totalRevenue)}
                  </td>
                  <td className="px-3 py-2 text-right font-mono text-amber-600">
                    {formatIDR(totalTax)}
                  </td>
                </tr>
              </tfoot>
            </table>
          </div>
        </>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Verify TypeScript**

Run: `cd frontend && npx tsc --noEmit`
Expected: exit 0

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/TaxSummaryCard.tsx
git commit -m "feat: add TaxSummaryCard with monthly tax table and PDF/Excel download buttons"
```

---

### Task 8: Integrate Dashboard into ProtectedInvoiceDashboard

**Files:**
- Modify: `frontend/src/components/ProtectedInvoiceDashboard.tsx`

- [ ] **Step 1: Rewrite `ProtectedInvoiceDashboard.tsx`** to add dashboard section above the existing content

The existing file is at `frontend/src/components/ProtectedInvoiceDashboard.tsx`. We add:
- Analytics state (overview, revenue, topClients, taxSummary, loading, year)
- `fetchAnalytics()` that fetches all 4 endpoints in parallel
- Dashboard section (cards + 2-column chart grid + tax table) above the form
- Year state lifted to dashboard level (shared between RevenueChart and TaxSummaryCard)

```tsx
import { useState, useEffect, useCallback } from "react";
import { Navbar } from "./Navbar";
import InvoiceForm from "./InvoiceForm";
import InvoicePreview from "./InvoicePreview";
import { DashboardCards } from "./DashboardCards";
import { RevenueChart } from "./RevenueChart";
import { TopClientsChart } from "./TopClientsChart";
import { TaxSummaryCard } from "./TaxSummaryCard";
import { apiFetch } from "../utils/api";
import { downloadFile } from "../utils/export";
import { User } from "../types/auth";
import type { Invoice } from "../types/invoice";
import type {
  AnalyticsOverview,
  RevenueDataPoint,
  TopClientData,
  TaxDataPoint,
} from "../types/analytics";

interface ProtectedInvoiceDashboardProps {
  user: User | null;
  onLogout: () => void;
}

const currentYear = new Date().getFullYear();

export function ProtectedInvoiceDashboard({
  user,
  onLogout,
}: ProtectedInvoiceDashboardProps) {
  // Legacy state (existing)
  const [savedInvoices, setSavedInvoices] = useState<Invoice[]>([]);
  const [preview, setPreview] = useState<Invoice | null>(null);
  const [loading, setLoading] = useState(true);
  const [exporting, setExporting] = useState<string | null>(null);

  // Analytics state (new)
  const [analyticsLoading, setAnalyticsLoading] = useState(true);
  const [overview, setOverview] = useState<AnalyticsOverview | null>(null);
  const [revenue, setRevenue] = useState<RevenueDataPoint[]>([]);
  const [topClients, setTopClients] = useState<TopClientData[]>([]);
  const [taxSummary, setTaxSummary] = useState<TaxDataPoint[]>([]);
  const [selectedYear, setSelectedYear] = useState(currentYear);

  // Fetch saved invoices (existing)
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

  // Fetch analytics data
  const fetchAnalytics = useCallback(async (year: number) => {
    setAnalyticsLoading(true);
    try {
      const [ov, revResp, tcResp, taxResp] = await Promise.all([
        apiFetch<AnalyticsOverview>("/analytics/overview"),
        apiFetch<{ data: RevenueDataPoint[] }>(`/analytics/revenue?year=${year}`),
        apiFetch<{ clients: TopClientData[] }>("/analytics/top-clients?limit=5"),
        apiFetch<{ data: TaxDataPoint[] }>(`/analytics/tax-summary?year=${year}`),
      ]);
      setOverview(ov);
      setRevenue(revResp.data || []);
      setTopClients(tcResp.clients || []);
      setTaxSummary(taxResp.data || []);
    } catch (err) {
      console.error("Failed to fetch analytics:", err);
    } finally {
      setAnalyticsLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchAnalytics(selectedYear);
  }, [fetchAnalytics, selectedYear]);

  // Legacy handlers (existing)
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

        {/* ── Dashboard Section ─────────────────────────────────── */}
        <section className="mb-10">
          <h2 className="text-2xl font-bold text-gray-800 mb-4">📊 Dashboard</h2>

          {/* Metric Cards */}
          <DashboardCards data={overview} loading={analyticsLoading} />

          {/* Charts Row */}
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-6 mb-6">
            <RevenueChart
              data={revenue}
              loading={analyticsLoading}
              year={selectedYear}
              onYearChange={setSelectedYear}
            />
            <TopClientsChart data={topClients} loading={analyticsLoading} />
          </div>

          {/* Tax Summary */}
          <TaxSummaryCard data={taxSummary} loading={analyticsLoading} year={selectedYear} />
        </section>

        {/* ── Invoice Section (existing) ────────────────────────── */}
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-8 mb-12">
          <section className="bg-white rounded-xl shadow p-6">
            <InvoiceForm onSaved={handleSaved} />
          </section>

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

        {/* Saved Invoices (existing) */}
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
```

- [ ] **Step 2: Verify full frontend build**

Run: `cd frontend && npm run build`
Expected: exit 0, Vite build succeeds

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/ProtectedInvoiceDashboard.tsx
git commit -m "feat: integrate analytics dashboard section into main page"
```

---

### Task 9: End-to-End Verification

- [ ] **Step 1: Full backend build**

Run: `cd backend && go build ./...`
Expected: exit 0

- [ ] **Step 2: Full frontend type check + build**

Run: `cd frontend && npx tsc --noEmit && npm run build`
Expected: exit 0 for both

- [ ] **Step 3: Verify .gitignore for superpowers**

Check `.gitignore` includes `.superpowers/`. If not, add it.

Run: `grep -q ".superpowers" /home/sekuyy/project/invoice-maker/.gitignore || echo ".superpowers/" >> /home/sekuyy/project/invoice-maker/.gitignore`

- [ ] **Step 4: Final commit**

```bash
git add .gitignore
git commit -m "chore: add .superpowers/ to gitignore"
```

---

## Verification Checklist (from spec)

- [ ] `go build ./...` — compile tanpa error
- [ ] `GET /api/analytics/overview` → 200 dengan data benar
- [ ] `GET /api/analytics/revenue?year=2026` → 200, array bulanan
- [ ] `GET /api/analytics/top-clients?limit=5` → 200, top clients
- [ ] `GET /api/analytics/tax-summary?year=2026` → 200, tax data
- [ ] `GET /api/analytics/report?format=pdf` → 200, file PDF valid
- [ ] `GET /api/analytics/report?format=excel` → 200, file Excel valid
- [ ] Semua endpoint dilindungi JWT (401 tanpa token)
- [ ] Isolasi data per user — hanya return data user sendiri
- [ ] `npm run build` — Vite build sukses
- [ ] `tsc --noEmit` — TypeScript strict lulus
- [ ] Dashboard cards render dengan data dari API
- [ ] Revenue chart render bar bulanan
- [ ] Top clients chart render pie
- [ ] Tax summary table + download buttons berfungsi
- [ ] Year selector ganti data di revenue chart + tax table
- [ ] Empty state tampil saat tidak ada data
