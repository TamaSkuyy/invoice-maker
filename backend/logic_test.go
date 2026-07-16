package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestHashPassword(t *testing.T) {
	hash, err := hashPassword("secret_password")
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
	assert.NotEqual(t, "secret_password", hash)

	// bcrypt salts each hash, so hashing the same password twice
	// must produce two different hashes.
	hash2, err := hashPassword("secret_password")
	require.NoError(t, err)
	assert.NotEqual(t, hash, hash2)
}

func TestVerifyHashPassword(t *testing.T) {
	hash, err := hashPassword("correct_password")
	require.NoError(t, err)

	assert.True(t, verifyPassword(hash, "correct_password"))
	assert.False(t, verifyPassword(hash, "incorrect_password"))
	assert.False(t, verifyPassword(hash, ""))
}

func TestGenerateAndValidateJWT(t *testing.T) {
	t.Run("valid token round-trips claims", func(t *testing.T) {
		token, err := generateJWT("sekuyy", "sekuyy@mail.com")
		require.NoError(t, err)
		assert.NotEmpty(t, token)

		claims, err := validateJWT(token)
		require.NoError(t, err)
		assert.Equal(t, "sekuyy",  claims.UserID)
		assert.Equal(t, "sekuyy@mail.com", claims.Email)
	})

	t.Run("malformed token is rejected", func(t *testing.T) {
		_, err := validateJWT("not_real_token")
		assert.Error(t, err)
	})

	t.Run("tampered token is rejected", func(t *testing.T) {
		token, err := generateJWT("sekuyy", "sekuyy@mail.com")
		require.NoError(t, err)

		tampered := token[:len(token)-2] + "xx"
		_, err = validateJWT(tampered)
		assert.Error(t, err)
	})

	t.Run("expired token is rejected", func(t *testing.T) {
		t.Setenv("JWT_EXPIRATION", "-1")
		token, err := generateJWT("sekuyy", "sekuyy@mail.com")
		require.NoError(t, err)

		_, err = validateJWT(token)
		assert.Error(t, err)
	})
}
