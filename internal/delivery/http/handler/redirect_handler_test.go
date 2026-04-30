package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/IndraSty/url-shortener-golang/internal/delivery/http/handler"
	"github.com/IndraSty/url-shortener-golang/internal/domain"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Mock redirect usecase
// ---------------------------------------------------------------------------

type mockRedirectUsecase struct {
	redirectFn          func(ctx context.Context, input domain.RedirectInput) (*domain.RedirectResult, error)
	unlockFn            func(ctx context.Context, slug string, password string) (*domain.RedirectResult, error)
	publishClickEventFn func(ctx context.Context, payload domain.ClickEventPayload) error
}

func (m *mockRedirectUsecase) Redirect(ctx context.Context, input domain.RedirectInput) (*domain.RedirectResult, error) {
	return m.redirectFn(ctx, input)
}

func (m *mockRedirectUsecase) UnlockWithPassword(ctx context.Context, slug string, password string) (*domain.RedirectResult, error) {
	return m.unlockFn(ctx, slug, password)
}

func (m *mockRedirectUsecase) PublishClickEvent(ctx context.Context, payload domain.ClickEventPayload) error {
	if m.publishClickEventFn != nil {
		return m.publishClickEventFn(ctx, payload)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Handler tests
// ---------------------------------------------------------------------------

func TestRedirectHandler_Success301(t *testing.T) {
	e := echo.New()

	mock := &mockRedirectUsecase{
		redirectFn: func(_ context.Context, input domain.RedirectInput) (*domain.RedirectResult, error) {
			assert.Equal(t, "abc123", input.Slug)
			return &domain.RedirectResult{
				DestinationURL: "https://example.com",
				StatusCode:     http.StatusMovedPermanently,
				LinkID:         1,
			}, nil
		},
	}

	h := handler.NewRedirectHandler(mock)

	req := httptest.NewRequest(http.MethodGet, "/abc123", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("slug")
	c.SetParamValues("abc123")

	err := h.Redirect(c)
	require.NoError(t, err)

	assert.Equal(t, http.StatusMovedPermanently, rec.Code)
	assert.Equal(t, "https://example.com", rec.Header().Get("Location"))
}

func TestRedirectHandler_Success302_ABTest(t *testing.T) {
	e := echo.New()

	mock := &mockRedirectUsecase{
		redirectFn: func(_ context.Context, _ domain.RedirectInput) (*domain.RedirectResult, error) {
			return &domain.RedirectResult{
				DestinationURL: "https://variant.example.com",
				StatusCode:     http.StatusFound,
				ABTestID:       "variant-uuid",
				LinkID:         2,
			}, nil
		},
	}

	h := handler.NewRedirectHandler(mock)

	req := httptest.NewRequest(http.MethodGet, "/myslug", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("slug")
	c.SetParamValues("myslug")

	err := h.Redirect(c)
	require.NoError(t, err)

	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "https://variant.example.com", rec.Header().Get("Location"))
}

func TestRedirectHandler_NotFound(t *testing.T) {
	e := echo.New()

	mock := &mockRedirectUsecase{
		redirectFn: func(_ context.Context, _ domain.RedirectInput) (*domain.RedirectResult, error) {
			return nil, domain.ErrLinkNotFound
		},
	}

	h := handler.NewRedirectHandler(mock)

	req := httptest.NewRequest(http.MethodGet, "/notexist", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("slug")
	c.SetParamValues("notexist")

	err := h.Redirect(c)

	// Handler returns the domain error — router's error handler maps it to 404
	assert.ErrorIs(t, err, domain.ErrLinkNotFound)
}

func TestRedirectHandler_PasswordRequired(t *testing.T) {
	e := echo.New()

	mock := &mockRedirectUsecase{
		redirectFn: func(_ context.Context, _ domain.RedirectInput) (*domain.RedirectResult, error) {
			return nil, domain.ErrPasswordRequired
		},
	}

	h := handler.NewRedirectHandler(mock)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("slug")
	c.SetParamValues("protected")

	err := h.Redirect(c)
	assert.ErrorIs(t, err, domain.ErrPasswordRequired)
}

func TestRedirectHandler_UnlockWithPassword_Success(t *testing.T) {
	e := echo.New()

	mock := &mockRedirectUsecase{
		unlockFn: func(_ context.Context, slug string, password string) (*domain.RedirectResult, error) {
			assert.Equal(t, "protected", slug)
			assert.Equal(t, "secret123", password)
			return &domain.RedirectResult{
				DestinationURL: "https://secret.example.com",
				StatusCode:     http.StatusFound,
				LinkID:         3,
			}, nil
		},
	}

	h := handler.NewRedirectHandler(mock)

	body := `{"password":"secret123"}`
	req := httptest.NewRequest(http.MethodPost, "/protected/unlock", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("slug")
	c.SetParamValues("protected")

	err := h.UnlockWithPassword(c)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "https://secret.example.com", resp["destination_url"])
}

func TestRedirectHandler_UnlockWithPassword_WrongPassword(t *testing.T) {
	e := echo.New()

	mock := &mockRedirectUsecase{
		unlockFn: func(_ context.Context, _ string, _ string) (*domain.RedirectResult, error) {
			return nil, domain.ErrInvalidPassword
		},
	}

	h := handler.NewRedirectHandler(mock)

	body := `{"password":"wrongpassword"}`
	req := httptest.NewRequest(http.MethodPost, "/protected/unlock", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("slug")
	c.SetParamValues("protected")

	err := h.UnlockWithPassword(c)
	assert.ErrorIs(t, err, domain.ErrInvalidPassword)
}

// ---------------------------------------------------------------------------
// Benchmark: handler overhead (without network I/O)
// ---------------------------------------------------------------------------

func BenchmarkRedirectHandler(b *testing.B) {
	e := echo.New()

	mock := &mockRedirectUsecase{
		redirectFn: func(_ context.Context, _ domain.RedirectInput) (*domain.RedirectResult, error) {
			return &domain.RedirectResult{
				DestinationURL: "https://example.com",
				StatusCode:     http.StatusMovedPermanently,
				LinkID:         1,
			}, nil
		},
	}

	h := handler.NewRedirectHandler(mock)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/bench", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("slug")
		c.SetParamValues("bench")
		h.Redirect(c) //nolint:errcheck
	}
}
