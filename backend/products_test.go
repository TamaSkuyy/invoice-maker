package main

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProductCreateAndList(t *testing.T) {
	truncateTables(t)
	router := setupRouter()
	token := registerTestUser(t, router, "productowner@mail.com", "password")

	createRec := doRequest(router, http.MethodPost, "/api/products", Product{
		Name: "Web Design",
		Description: "Custom website design",
		DefaultPrice: 500,
	}, token)
	require.Equal(t, http.StatusCreated, createRec.Code, createRec.Body.String())

	var created Product
	require.NoError(t, decodeJSON(createRec.Body.Bytes(), &created))
	assert.NotEmpty(t, created.ID)
	assert.Equal(t, 500.0, created.DefaultPrice)

	listRec := doRequest(router, http.MethodGet, "/api/products", nil, token)
	require.Equal(t, http.StatusOK, listRec.Code)

	var products []Product
	require.NoError(t, decodeJSON(listRec.Body.Bytes(), &products))
	require.Len(t, products, 1)
	assert.Equal(t, "Web Design", products[0].Name)
}

func TestProductDeleteNotFound(t *testing.T) {
	truncateTables(t)
	router := setupRouter()
	token := registerTestUser(t, router, "productowner2@mail.com", "password")

	rec := doRequest(router, http.MethodDelete, "/api/products/000000-00-00-0000", nil, token)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}