package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePagination(t *testing.T) {
	t.Run("defaults when no params", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/items", nil)
		p := parsePagination(r)
		assert.Equal(t, 1, p.Page)
		assert.Equal(t, 50, p.PerPage)
	})

	t.Run("custom values", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/items?page=3&per_page=25", nil)
		p := parsePagination(r)
		assert.Equal(t, 3, p.Page)
		assert.Equal(t, 25, p.PerPage)
	})

	t.Run("negative page falls back to default", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/items?page=-1&per_page=10", nil)
		p := parsePagination(r)
		assert.Equal(t, 1, p.Page, "negative page should fall back to default 1")
		assert.Equal(t, 10, p.PerPage)
	})

	t.Run("zero page falls back to default", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/items?page=0", nil)
		p := parsePagination(r)
		assert.Equal(t, 1, p.Page, "zero page should fall back to default 1")
	})

	t.Run("per_page exceeding max clamped to default", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/items?per_page=200", nil)
		p := parsePagination(r)
		assert.Equal(t, 50, p.PerPage, "per_page > 100 should fall back to default 50")
	})

	t.Run("per_page at max boundary", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/items?per_page=100", nil)
		p := parsePagination(r)
		assert.Equal(t, 100, p.PerPage, "per_page=100 is the max allowed")
	})

	t.Run("non-numeric values fall back to defaults", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/items?page=abc&per_page=xyz", nil)
		p := parsePagination(r)
		assert.Equal(t, 1, p.Page)
		assert.Equal(t, 50, p.PerPage)
	})

	t.Run("negative per_page falls back to default", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/items?per_page=-5", nil)
		p := parsePagination(r)
		assert.Equal(t, 50, p.PerPage, "negative per_page should fall back to default 50")
	})
}

func TestParseUUID(t *testing.T) {
	t.Run("valid UUID", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "550e8400-e29b-41d4-a716-446655440000")
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))

		id, err := parseUUID(r, "id")
		require.NoError(t, err)
		assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", id.String())
	})

	t.Run("invalid UUID returns error", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "not-a-uuid")
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))

		_, err := parseUUID(r, "id")
		assert.Error(t, err)
	})

	t.Run("empty param returns error", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "")
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))

		_, err := parseUUID(r, "id")
		assert.Error(t, err)
	})
}

func TestKillSwitch(t *testing.T) {
	t.Run("default state is disengaged", func(t *testing.T) {
		h := &Handler{}
		assert.False(t, h.IsKillSwitchEngaged())
	})

	t.Run("engage kill switch", func(t *testing.T) {
		h := &Handler{}
		h.SetKillSwitch(true)
		assert.True(t, h.IsKillSwitchEngaged())
	})

	t.Run("disengage kill switch", func(t *testing.T) {
		h := &Handler{}
		h.SetKillSwitch(true)
		assert.True(t, h.IsKillSwitchEngaged())

		h.SetKillSwitch(false)
		assert.False(t, h.IsKillSwitchEngaged())
	})

	t.Run("toggle multiple times", func(t *testing.T) {
		h := &Handler{}
		for i := 0; i < 5; i++ {
			h.SetKillSwitch(true)
			assert.True(t, h.IsKillSwitchEngaged())
			h.SetKillSwitch(false)
			assert.False(t, h.IsKillSwitchEngaged())
		}
	})
}
