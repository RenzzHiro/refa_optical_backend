package handlers

import (
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/refa-optical/backend/internal/auth"
	"github.com/refa-optical/backend/internal/ids"
	"github.com/refa-optical/backend/internal/middleware"
	"github.com/refa-optical/backend/internal/models"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"net/http"
	"strings"
	"time"
)

type authInput struct {
	Name     string `json:"name"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
}
type authUser struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Role  string `json:"role"`
}
type authResponse struct {
	AccessToken string   `json:"accessToken"`
	TokenType   string   `json:"tokenType"`
	ExpiresIn   int64    `json:"expiresIn"`
	User        authUser `json:"user"`
}

func (s *Server) Register(c *gin.Context) {
	var input authInput
	if err := c.ShouldBindJSON(&input); err != nil {
		s.fail(c, 400, "invalid_request", "Nama, email valid, dan password minimal 8 karakter diperlukan.")
		return
	}
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		s.fail(c, 400, "invalid_request", "Nama wajib diisi.")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		s.fail(c, 500, "internal_error", "Gagal membuat akun.")
		return
	}
	user := models.User{ID: ids.New(), Name: input.Name, Email: input.Email, PasswordHash: string(hash), Role: "customer"}
	if err = s.DB.Create(&user).Error; err != nil {
		s.fail(c, 409, "email_exists", "Email sudah terdaftar.")
		return
	}
	s.issueTokens(c, user, http.StatusCreated)
}
func (s *Server) Login(c *gin.Context) {
	var input authInput
	if err := c.ShouldBindJSON(&input); err != nil {
		s.fail(c, 400, "invalid_request", "Email dan password diperlukan.")
		return
	}
	var user models.User
	err := s.DB.Where("email = ?", strings.ToLower(strings.TrimSpace(input.Email))).First(&user).Error
	if err != nil || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.Password)) != nil {
		s.fail(c, 401, "invalid_credentials", "Email atau password tidak cocok.")
		return
	}
	s.issueTokens(c, user, http.StatusOK)
}
func (s *Server) issueTokens(c *gin.Context, user models.User, status int) {
	access, err := auth.IssueAccess(user, s.Config.JWTSecret, s.Config.AccessTokenTTL)
	if err != nil {
		s.fail(c, 500, "token_error", "Gagal membuat token akses.")
		return
	}
	raw, hash, err := auth.NewRefreshToken()
	if err != nil {
		s.fail(c, 500, "token_error", "Gagal membuat refresh token.")
		return
	}
	record := models.RefreshToken{ID: ids.New(), UserID: user.ID, TokenHash: hash, ExpiresAt: time.Now().Add(s.Config.RefreshTokenTTL)}
	if err = s.DB.Create(&record).Error; err != nil {
		s.fail(c, 500, "token_error", "Gagal menyimpan sesi.")
		return
	}
	s.setRefreshCookie(c, raw)
	c.JSON(status, gin.H{"data": authResponse{AccessToken: access, TokenType: "Bearer", ExpiresIn: int64(s.Config.AccessTokenTTL.Seconds()), User: authUser{ID: user.ID, Name: user.Name, Email: user.Email, Role: user.Role}}})
}
func (s *Server) setRefreshCookie(c *gin.Context, value string) {
	http.SetCookie(c.Writer, &http.Cookie{Name: "refa_refresh", Value: value, Path: "/api/v1/auth", HttpOnly: true, Secure: s.Config.CookieSecure, SameSite: s.refreshCookieSameSite(), Expires: time.Now().Add(s.Config.RefreshTokenTTL), MaxAge: int(s.Config.RefreshTokenTTL.Seconds())})
}

func (s *Server) refreshCookieSameSite() http.SameSite {
	if s.Config.CookieSecure {
		return http.SameSiteNoneMode
	}
	return http.SameSiteLaxMode
}

func (s *Server) Refresh(c *gin.Context) {
	cookie, err := c.Cookie("refa_refresh")
	if err != nil {
		s.fail(c, 401, "refresh_required", "Sesi refresh tidak ditemukan.")
		return
	}
	hash := auth.HashRefreshToken(cookie)
	var old models.RefreshToken
	if err = s.DB.Where("token_hash = ? AND revoked_at IS NULL AND expires_at > ?", hash, time.Now()).First(&old).Error; err != nil {
		s.fail(c, 401, "refresh_invalid", "Sesi refresh tidak valid atau kedaluwarsa.")
		return
	}
	var user models.User
	if err = s.DB.First(&user, "id = ?", old.UserID).Error; err != nil {
		s.fail(c, 401, "refresh_invalid", "Akun tidak ditemukan.")
		return
	}
	now := time.Now()
	old.RevokedAt = &now
	if err = s.DB.Save(&old).Error; err != nil {
		s.fail(c, 500, "token_error", "Gagal memperbarui sesi.")
		return
	}
	s.issueTokens(c, user, http.StatusOK)
}
func (s *Server) Logout(c *gin.Context) {
	if cookie, err := c.Cookie("refa_refresh"); err == nil {
		hash := auth.HashRefreshToken(cookie)
		now := time.Now()
		_ = s.DB.Model(&models.RefreshToken{}).Where("token_hash = ? AND revoked_at IS NULL", hash).Update("revoked_at", now).Error
	}
	http.SetCookie(c.Writer, &http.Cookie{Name: "refa_refresh", Value: "", Path: "/api/v1/auth", HttpOnly: true, Secure: s.Config.CookieSecure, SameSite: s.refreshCookieSameSite(), MaxAge: -1, Expires: time.Unix(1, 0)})
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"message": "Berhasil keluar."}})
}
func (s *Server) Me(c *gin.Context) {
	var user models.User
	if err := s.DB.First(&user, "id = ?", s.userID(c)).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			s.fail(c, 404, "not_found", "Akun tidak ditemukan.")
			return
		}
		s.fail(c, 500, "database_error", "Gagal membaca akun.")
		return
	}
	s.ok(c, authUser{ID: user.ID, Name: user.Name, Email: user.Email, Role: user.Role})
}
func (s *Server) CreateAdminIfConfigured(email, password string) error {
	if email == "" || password == "" {
		return nil
	}
	email = strings.ToLower(strings.TrimSpace(email))
	var existing models.User
	if err := s.DB.Where("email = ?", email).First(&existing).Error; err == nil {
		return s.DB.Model(&existing).Update("role", "admin").Error
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.DB.Create(&models.User{ID: ids.New(), Name: "Refa Admin", Email: email, PasswordHash: string(hash), Role: "admin"}).Error
}
func (s *Server) CurrentUser(c *gin.Context) (models.User, error) {
	var user models.User
	err := s.DB.First(&user, "id = ?", c.GetString(middleware.UserIDKey)).Error
	return user, err
}
