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
	TotalRevenue    float64 `json:"total_revenue"`
	TotalInvoices   int     `json:"total_invoices"`
	TotalClients    int     `json:"total_clients"`
	AvgInvoiceValue float64 `json:"avg_invoice_value"`
	PaidAmount		float64	`json:"paid_amount"`
	PendingAmount	float64	`json:"pending_amount"`
	OverdueCount	int		`json:"overdue_count"`
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
		var month float64
		if err := rows.Scan(&d.Label, &month, &d.Total, &d.Count); err != nil {
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
		GROUP BY COALESCE(c.name, i.client_name)
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
		var month float64
		if err := rows.Scan(&d.Label, &month, &d.Tax, &d.Revenue); err != nil {
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

	// Gather overview
	var overview AnalyticsOverview
	db.QueryRow(ctx, `
		SELECT COALESCE(SUM(total_amount),0), COUNT(*),
			COUNT(DISTINCT client_id),
			CASE WHEN COUNT(*)>0 THEN SUM(total_amount)/COUNT(*) ELSE 0 END
		FROM invoices WHERE user_id=$1
	`, userID).Scan(&overview.TotalRevenue, &overview.TotalInvoices, &overview.TotalClients, &overview.AvgInvoiceValue)
	overview.AvgInvoiceValue = round2(overview.AvgInvoiceValue)

	// Gather revenue
	revRows, _ := db.Query(ctx, `
		SELECT TO_CHAR(date,'Mon'), EXTRACT(MONTH FROM date),
			COALESCE(SUM(total_amount),0), COUNT(*)
		FROM invoices WHERE user_id=$1 AND EXTRACT(YEAR FROM date)=$2
		GROUP BY month, label ORDER BY month
	`, userID, year)

	var revenue []RevenueDataPoint
	for revRows.Next() {
		var d RevenueDataPoint
		var m float64
		revRows.Scan(&d.Label, &m, &d.Total, &d.Count)
		d.Total = round2(d.Total)
		revenue = append(revenue, d)
	}
	revRows.Close()

	// Gather tax data
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
		var m float64
		taxRows.Scan(&d.Label, &m, &d.Tax, &d.Revenue)
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

	// Title header
	pdf.SetFillColor(headerColor[0], headerColor[1], headerColor[2])
	pdf.Rect(0, 0, pageW, 36, "F")
	pdf.SetFont("Helvetica", "B", 22)
	pdf.SetTextColor(255, 255, 255)
	pdf.SetY(10)
	pdf.CellFormat(0, 12, fmt.Sprintf("Financial Report - %d", year), "", 1, "C", false, 0, "")
	pdf.Ln(16)

	// Overview
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

	// Monthly breakdown
	pdf.SetFont("Helvetica", "B", 12)
	pdf.SetTextColor(darkColor[0], darkColor[1], darkColor[2])
	pdf.Cell(0, 8, "Monthly Breakdown")
	pdf.Ln(10)

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
	pdf.CellFormat(0, 4, fmt.Sprintf("Generated by Invoice Maker - %s", time.Now().Format("2006-01-02 15:04")), "", 0, "C", false, 0, "")

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
	moneyFmt := "\"Rp \"#,##0.00"
	moneyStyle, _ := f.NewStyle(&excelize.Style{
		CustomNumFmt: &moneyFmt,
		Alignment: &excelize.Alignment{Horizontal: "right"},
	})
	centerStyle, _ := f.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})

	// Sheet 1: Overview
	overviewSheet := "Overview"
	f.SetSheetName("Sheet1", overviewSheet)
	f.SetCellValue(overviewSheet, "A1", fmt.Sprintf("Financial Report - %d", year))
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
