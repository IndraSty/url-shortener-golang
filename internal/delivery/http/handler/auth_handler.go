package handler

import (
	"net/http"

	"github.com/IndraSty/url-shortener-golang/internal/domain"
	"github.com/IndraSty/url-shortener-golang/pkg/logger"
	"github.com/labstack/echo/v4"
)

type AuthHandler struct {
	authUsecase domain.AuthUsecase
}

func NewAuthHandler(authUsecase domain.AuthUsecase) *AuthHandler {
	return &AuthHandler{authUsecase: authUsecase}
}

// Register godoc
// @Summary      Register a new user
// @Description  Creates a new account and returns JWT tokens + API key (shown once)
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body body registerRequest true "Registration payload"
// @Success      201  {object} registerResponse
// @Failure      400  {object} errorResponse
// @Failure      409  {object} errorResponse
// @Router       /api/v1/auth/register [post]
func (h *AuthHandler) Register(c echo.Context) error {
	var req registerRequest
	if err := bindAndValidate(c, &req); err != nil {
		return err
	}

	tokens, err := h.authUsecase.Register(c.Request().Context(), domain.RegisterInput{
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		return err
	}

	// The access token contains the raw API key appended with "|apikey:" prefix.
	// We parse it out here so the handler can return it separately.
	// This is the ONLY time the plaintext API key is visible.
	accessToken, rawAPIKey := splitAPIKeyFromToken(tokens.AccessToken)

	log := logger.GetLogger(c)
	log.Info().Str("email", req.Email).Msg("user registered")

	return c.JSON(http.StatusCreated, registerResponse{
		AccessToken:  accessToken,
		RefreshToken: tokens.RefreshToken,
		ExpiresIn:    tokens.ExpiresIn,
		APIKey:       rawAPIKey,
		Message:      "Save your API key — it will not be shown again",
	})
}

// Login godoc
// @Summary      Login
// @Description  Authenticates a user and returns JWT tokens
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body body loginRequest true "Login payload"
// @Success      200  {object} loginResponse
// @Failure      401  {object} errorResponse
// @Router       /api/v1/auth/login [post]
func (h *AuthHandler) Login(c echo.Context) error {
	var req loginRequest
	if err := bindAndValidate(c, &req); err != nil {
		return err
	}

	tokens, err := h.authUsecase.Login(c.Request().Context(), domain.LoginInput{
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		return err
	}

	log := logger.GetLogger(c)
	log.Info().Str("email", req.Email).Msg("user logged in")

	return c.JSON(http.StatusOK, loginResponse{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		ExpiresIn:    tokens.ExpiresIn,
	})
}

// --- Request / Response types ---

type registerRequest struct {
	Email    string `json:"email"    validate:"required,email"`
	Password string `json:"password" validate:"required,min=8"`
}

type loginRequest struct {
	Email    string `json:"email"    validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

type registerResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	APIKey       string `json:"api_key"`
	Message      string `json:"message"`
}

type loginResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}
