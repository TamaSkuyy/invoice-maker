package main

import (
	"bytes"
	"encoding/csv"
	"fmt"

	"github.com/xuri/excelize/v2"
)

func generateInvoicesExcel(invoices []Invoice) ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()

	sheet := "Invoices"
	f.SetSheetName("Sheet1", sheet)

	// ── Header style ────────────────────────────────────────────────────
	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 11, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"2563EB"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border: []excelize.Border{
			{Type: "bottom", Color: "1E3A8A", Style: 2},
		},
	})

	moneyStyle, _ := f.NewStyle(&excelize.Style{
		NumFmt:    36, // $#,##0.00
		Alignment: &excelize.Alignment{Horizontal: "right"},
	})

	dateStyle, _ := f.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})

	centerStyle, _ := f.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})

	// ── Headers ─────────────────────────────────────────────────────────
	headers := []string{"Invoice ID", "Client", "Date", "Items", "Tax Rate", "Total Amount", "Created At"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}
	f.SetRowHeight(sheet, 1, 28)

	// ── Data rows ───────────────────────────────────────────────────────
	for rowIdx, inv := range invoices {
		row := rowIdx + 2
		itemCount := fmt.Sprintf("%d", len(inv.Items))
		taxLabel := fmt.Sprintf("%.1f%%", inv.TaxRate)

		f.SetCellValue(sheet, cellName(1, row), inv.ID)
		f.SetCellValue(sheet, cellName(2, row), inv.ClientName)
		f.SetCellValue(sheet, cellName(3, row), inv.Date)
		f.SetCellValue(sheet, cellName(4, row), itemCount)
		f.SetCellValue(sheet, cellName(5, row), taxLabel)
		f.SetCellValue(sheet, cellName(6, row), inv.TotalAmount)
		f.SetCellValue(sheet, cellName(7, row), inv.CreatedAt.Format("2006-01-02 15:04"))
	}

	// ── Column widths ───────────────────────────────────────────────────
	f.SetColWidth(sheet, "A", "A", 38)
	f.SetColWidth(sheet, "B", "B", 28)
	f.SetColWidth(sheet, "C", "C", 14)
	f.SetColWidth(sheet, "D", "D", 10)
	f.SetColWidth(sheet, "E", "E", 12)
	f.SetColWidth(sheet, "F", "F", 16)
	f.SetColWidth(sheet, "G", "G", 18)

	// ── Apply styles ────────────────────────────────────────────────────
	lastRow := len(invoices) + 1
	for col := 1; col <= 7; col++ {
		f.SetCellStyle(sheet, cellName(col, 1), cellName(col, 1), headerStyle)
	}
	for row := 2; row <= lastRow; row++ {
		f.SetCellStyle(sheet, cellName(6, row), cellName(6, row), moneyStyle)
		f.SetCellStyle(sheet, cellName(3, row), cellName(3, row), dateStyle)
		f.SetCellStyle(sheet, cellName(4, row), cellName(5, row), centerStyle)
	}

	f.SetSheetRow(sheet, "A1", &headers)

	// ── Auto-filter ─────────────────────────────────────────────────────
	lastCell, _ := excelize.CoordinatesToCellName(7, lastRow)
	ref := fmt.Sprintf("A1:%s", lastCell)
	f.AutoFilter(sheet, ref, []excelize.AutoFilterOptions{})

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("failed to generate Excel: %w", err)
	}
	return buf.Bytes(), nil
}

func generateInvoiceCSV(inv Invoice) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)

	// Header
	w.Write([]string{"Invoice ID", inv.ID})
	w.Write([]string{"Client", inv.ClientName})
	w.Write([]string{"Date", inv.Date})
	w.Write([]string{"Tax Rate", fmt.Sprintf("%.1f%%", inv.TaxRate)})
	w.Write([]string{""})

	// Items table
	w.Write([]string{"Description", "Qty", "Unit Price", "Amount"})
	var subtotal float64
	for _, item := range inv.Items {
		amount := item.Qty * item.Price
		subtotal += amount
		w.Write([]string{
			item.Description,
			fmt.Sprintf("%.0f", item.Qty),
			fmt.Sprintf("%.2f", item.Price),
			fmt.Sprintf("%.2f", amount),
		})
	}
	w.Write([]string{""})
	w.Write([]string{"Subtotal", "", "", fmt.Sprintf("%.2f", subtotal)})
	taxAmount := subtotal * inv.TaxRate / 100
	w.Write([]string{fmt.Sprintf("Tax (%.1f%%)", inv.TaxRate), "", "", fmt.Sprintf("%.2f", taxAmount)})
	w.Write([]string{"Total", "", "", fmt.Sprintf("%.2f", inv.TotalAmount)})

	w.Flush()
	if err := w.Error(); err != nil {
		return nil, fmt.Errorf("failed to generate CSV: %w", err)
	}
	return buf.Bytes(), nil
}

func cellName(col, row int) string {
	name, _ := excelize.CoordinatesToCellName(col, row)
	return name
}
