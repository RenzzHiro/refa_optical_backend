package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/refa-optical/backend/internal/config"
	"github.com/refa-optical/backend/internal/payment"
	"github.com/refa-optical/backend/internal/storage"
	"gorm.io/gorm"
)

type Server struct {
	DB      *gorm.DB
	Config  config.Config
	Store   storage.Store
	Gateway *payment.Midtrans
}

func (s *Server) ok(c *gin.Context, data any) { c.JSON(200, gin.H{"data": data}) }
func (s *Server) fail(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{"error": gin.H{"code": code, "message": message}})
}
func (s *Server) userID(c *gin.Context) string {
	value, _ := c.Get("userID")
	id, _ := value.(string)
	return id
}
func (s *Server) userEmail(c *gin.Context) string {
	value, _ := c.Get("userEmail")
	email, _ := value.(string)
	return email
}
