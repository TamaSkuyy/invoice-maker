package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRound2(t *testing.T) {
	tests := []struct{
		name string
		in float64
		want float64
	}{
		{"Already 2 decimals", 10.20, 10.20},
		{"Need Rounding Up", 10.506, 10.51},
		{"Need Rounding Down", 10.504, 10.50},
		{"Already Int", 10, 10},
		{"Negative Value", -10.506, -10.51},
		{"Zero", 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T){
			assert.Equal(t, tt.want, round2(tt.in))
		})
	}
	
}

func TestCalculateTotal(t *testing.T) {
	tests := []struct{
		name string
		items []InvoiceItem
		taxRate float64
		want float64
	}{
		{
			name: "single item no tax", 
			items: []InvoiceItem{
				{Description: "Project 1", Qty: 1, Price: 100},
			},
			taxRate: 0,
			want: 100,
		},
		{
			name: "single item with tax",
			items: []InvoiceItem{
				{Description: "Project 1", Qty: 1, Price: 100},
			},
			taxRate: 10,
			want: 110,
		},
		{
			name: "multiple item with tax",
			items: []InvoiceItem{
				{Description: "Project 1", Qty: 1, Price: 100},
				{Description: "Project 2", Qty: 1, Price: 200},
				{Description: "Project 3", Qty: 1, Price: 300},
			},
			taxRate: 10,
			want: 660,
		},
		{
			name: "no items",
			items: []InvoiceItem{},
			taxRate: 10,
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T){
			assert.Equal(t, tt.want, calculateTotal(tt.items, tt.taxRate))
		})
	}
}
