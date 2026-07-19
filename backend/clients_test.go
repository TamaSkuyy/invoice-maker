package main

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientCreateAndList(t *testing.T) {
	truncateTables(t)
	router := setupRouter()
	token := registerTestUser(t, router, "clientowner@mail.com", "password")

	createRec := doRequest(router, http.MethodPost, "/api/clients", Client{
		Name: "Corp A",
		Email: "billing@corpa.com",
		Phone: "555-1234",
		Address: "123 Main St",
	}, token)
	require.Equal(t, http.StatusCreated, createRec.Code, createRec.Body.String())

	var created Client
	require.NoError(t, decodeJSON(createRec.Body.Bytes(), &created))
	assert.NotEmpty(t, created.ID)

	listRec := doRequest(router, http.MethodGet, "/api/clients", nil, token)
	require.Equal(t, http.StatusOK, listRec.Code)

	var clients []Client
	require.NoError(t, decodeJSON(listRec.Body.Bytes(), &clients))
	require.Len(t, clients, 1)
	assert.Equal(t, "Corp A", clients[0].Name)
}

func TestClientDeleteNotFound(t *testing.T) {
	truncateTables(t)
	router := setupRouter()
	token := registerTestUser(t, router, "clientowner2@mail.com", "password")

	rec := doRequest(router, http.MethodDelete, "/api/clients/00000000-0000-0000-0000-000000000000", nil, token)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}