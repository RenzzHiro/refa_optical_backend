package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/refa-optical/backend/internal/ids"
	"github.com/refa-optical/backend/internal/models"
	"gorm.io/gorm"
	"net/http"
	"strings"
)

func (s *Server) ListLenses(c *gin.Context) {
	var lenses []models.LensOption
	if err := s.DB.Where("is_active = ? AND stock > 0", true).Order("type ASC, price ASC").Find(&lenses).Error; err != nil {
		s.fail(c, 500, "database_error", "Pilihan lensa tidak dapat dimuat.")
		return
	}
	s.ok(c, lenses)
}

func (s *Server) ListAdminLenses(c *gin.Context) {
	var lenses []models.LensOption
	if err := s.DB.Order("type ASC, name ASC").Find(&lenses).Error; err != nil {
		s.fail(c, 500, "database_error", "Daftar lensa tidak dapat dimuat.")
		return
	}
	s.ok(c, lenses)
}

type lensInput struct {
	Name        string `json:"name" binding:"required,max=120"`
	Type        string `json:"type" binding:"required,oneof=minus anti_radiation photochromic"`
	Description string `json:"description"`
	Price       int64  `json:"price" binding:"gte=0"`
	Stock       int    `json:"stock" binding:"gte=0"`
	IsActive    *bool  `json:"isActive"`
}

func (s *Server) CreateLens(c *gin.Context) {
	var input lensInput
	if err := c.ShouldBindJSON(&input); err != nil {
		s.fail(c, 400, "invalid_request", "Data lensa tidak valid.")
		return
	}
	active := true
	if input.IsActive != nil { active = *input.IsActive }
	lens := models.LensOption{ID: ids.New(), Name: strings.TrimSpace(input.Name), Type: input.Type, Description: strings.TrimSpace(input.Description), Price: input.Price, Stock: input.Stock, RequiresPrescription: input.Type == "minus", IsActive: active}
	if err := s.DB.Create(&lens).Error; err != nil {
		s.fail(c, 500, "database_error", "Lensa tidak dapat disimpan.")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": lens})
}

func (s *Server) UpdateLens(c *gin.Context) {
	var input lensInput
	if err := c.ShouldBindJSON(&input); err != nil {
		s.fail(c, 400, "invalid_request", "Data lensa tidak valid.")
		return
	}
	var lens models.LensOption
	if err := s.DB.First(&lens, "id = ?", c.Param("id")).Error; err != nil {
		s.fail(c, 404, "not_found", "Lensa tidak ditemukan.")
		return
	}
	lens.Name = strings.TrimSpace(input.Name)
	lens.Type = input.Type
	lens.Description = strings.TrimSpace(input.Description)
	lens.Price = input.Price
	lens.Stock = input.Stock
	lens.RequiresPrescription = input.Type == "minus"
	if input.IsActive != nil { lens.IsActive = *input.IsActive }
	if err := s.DB.Save(&lens).Error; err != nil {
		s.fail(c, 500, "database_error", "Lensa tidak dapat diperbarui.")
		return
	}
	s.ok(c, lens)
}

func (s *Server) DeleteLens(c *gin.Context) {
	result := s.DB.Model(&models.LensOption{}).Where("id = ?", c.Param("id")).Update("is_active", false)
	if result.Error != nil {
		s.fail(c, 500, "database_error", "Lensa tidak dapat dinonaktifkan.")
		return
	}
	if result.RowsAffected == 0 {
		s.fail(c, 404, "not_found", "Lensa tidak ditemukan.")
		return
	}
	s.ok(c, gin.H{"message": "Lensa dinonaktifkan."})
}

func (s *Server) DeleteLensPermanently(c *gin.Context) {
	var lens models.LensOption
	if err := s.DB.First(&lens, "id = ?", c.Param("id")).Error; err != nil {
		s.fail(c, 404, "not_found", "Lensa tidak ditemukan.")
		return
	}
	var orderItemCount int64
	if err := s.DB.Model(&models.OrderItem{}).Where("lens_id = ?", lens.ID).Count(&orderItemCount).Error; err != nil {
		s.fail(c, 500, "database_error", "Riwayat pesanan lensa tidak dapat diperiksa.")
		return
	}
	if orderItemCount > 0 {
		s.fail(c, 409, "lens_in_use", "Lensa sudah tercatat di riwayat pesanan. Nonaktifkan lensa jika ingin menyembunyikannya.")
		return
	}
	if err := s.DB.Unscoped().Delete(&lens).Error; err != nil {
		s.fail(c, 409, "delete_failed", "Lensa tidak dapat dihapus.")
		return
	}
	s.ok(c, gin.H{"message": "Lensa dihapus permanen."})
}

func (s *Server) SetLensActive(c *gin.Context) {
	var input struct {
		IsActive *bool `json:"isActive" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil || input.IsActive == nil {
		s.fail(c, 400, "invalid_request", "Status aktif lensa tidak valid.")
		return
	}
	var lens models.LensOption
	if err := s.DB.First(&lens, "id = ?", c.Param("id")).Error; err != nil {
		s.fail(c, 404, "not_found", "Lensa tidak ditemukan.")
		return
	}
	if err := s.DB.Model(&lens).Update("is_active", *input.IsActive).Error; err != nil {
		s.fail(c, 500, "database_error", "Status lensa tidak dapat diperbarui.")
		return
	}
	s.ok(c, gin.H{"lensId": lens.ID, "isActive": *input.IsActive})
}

func (s *Server) AdjustLensStock(c *gin.Context) {
	var input stockInput
	if err := c.ShouldBindJSON(&input); err != nil || input.Delta == 0 {
		s.fail(c, 400, "invalid_request", "Perubahan stok harus berupa angka selain nol.")
		return
	}
	result := s.DB.Model(&models.LensOption{}).Where("id = ? AND stock + ? >= 0", c.Param("id"), input.Delta).Update("stock", gorm.Expr("stock + ?", input.Delta))
	if result.Error != nil {
		s.fail(c, 500, "database_error", "Stok lensa tidak dapat diperbarui.")
		return
	}
	if result.RowsAffected == 0 {
		s.fail(c, 409, "insufficient_stock", "Lensa tidak ditemukan atau stok tidak boleh negatif.")
		return
	}
	var lens models.LensOption
	_ = s.DB.First(&lens, "id = ?", c.Param("id")).Error
	s.ok(c, gin.H{"lensId": lens.ID, "stock": lens.Stock})
}
