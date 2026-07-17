package main

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegister(t *testing.T) {
	truncateTables(t)
	router := setupRouter()

	t.Run("happy path returns 201 with token", func(t *testing.T) {
		rec := doRequest(router, http.MethodPost, "/api/auth/register", SignupRequest{
			Email: "newuser@mail.com",
			Password: "password123",
		}, "")

		require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

		var resp AuthResponse
		require.NoError(t, decodeJSON(rec.Body.Bytes(), &resp))
		assert.NotEmpty(t, resp.Token)
		assert.Equal(t, "newuser@mail.com", resp.User.Email)
	})

	t.Run("duplicate emails return 400", func(t *testing.T) {
		registerTestUser(t, router, "dupe@mail.com", "password")

		rec := doRequest(router, http.MethodPost, "/api/auth/register", SignupRequest{
			Email: "dupe@mail.com",
			Password: "anotherone",
		}, "")

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

func TestLogin(t *testing.T) {
	truncateTables(t)
	router := setupRouter()
	registerTestUser(t, router, "logintest@mail.com", "correctpassword")

	t.Run("happy path returns 200 with token", func(t *testing.T) {
		rec := doRequest(router, http.MethodPost, "/api/auth/login", LoginRequest{
			Email: "logintest@mail.com",
			Password: "correctpassword",
		}, "")

		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

		var resp AuthResponse
		require.NoError(t, decodeJSON(rec.Body.Bytes(), &resp))
		assert.NotEmpty(t, resp.Token)
	})

	t.Run("wrong password returns 401", func(t *testing.T) {
		rec := doRequest(router, http.MethodPost, "/api/auth/login", LoginRequest{
			Email: "logintest@mail.com",
			Password: "wrongpassword",
		}, "")

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}

func TestProtectedRouteRequiresAuth(t *testing.T) {
	truncateTables(t)
	router := setupRouter()

	t.Run("no authorization header returns 401", func(t *testing.T) {
		rec := doRequest(router, http.MethodGet, "/api/invoices", nil, "")
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("invalid token returns 401", func(t *testing.T) {
		rec := doRequest(router, http.MethodGet, "/api/invoices", nil, "not-a-valid-token")
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}