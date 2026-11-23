package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/refa-optical/backend/internal/ids"
	"github.com/refa-optical/backend/internal/models"
	"gorm.io/gorm"
	"net/http"
	"regexp"
	"strings"
)

func (s *Server) ListProducts(c *gin.Context) {
	db := s.DB.Model(&models.Product{})
	if c.GetString("userRole") != "admin" {
		db = db.Where("is_active = ?", true)
	}
	query := strings.TrimSpace(c.Query("query"))
	if query == "" {
		query = strings.TrimSpace(c.Query("q"))
	}
	if query != "" {
		like := "%" + query + "%"
		db = db.Where("name LIKE ? OR description LIKE ?", like, like)
	}
	if value := c.Query("gender"); value != "" {
		db = db.Where("JSON_CONTAINS(gender, JSON_QUOTE(?))", value)
	}
	if value := c.Query("face_type"); value != "" {
		db = db.Where("JSON_CONTAINS(face_types, JSON_QUOTE(?))", value)
	}
	if value := c.Query("frame"); value != "" {
		db = db.Where("frame = ?", value)
	}
	if value := c.Query("category"); value != "" {
		db = db.Where("category = ?", value)
	}
	if value := c.Query("min_price"); value != "" {
		db = db.Where("price >= ?", value)
	}
	if value := c.Query("max_price"); value != "" {
		db = db.Where("price <= ?", value)
	}
	switch c.Query("sort") {
	case "price-asc":
		db = db.Order("price ASC")
	case "price-desc":
		db = db.Order("price DESC")
	case "newest":
		db = db.Order("created_at DESC")
	default:
		db = db.Order("is_new DESC, created_at DESC")
	}
	var products []models.Product
	if err := db.Find(&products).Error; err != nil {
		s.fail(c, 500, "database_error", "Gagal memuat katalog.")
		return
	}
	s.ok(c, products)
}
func (s *Server) GetProduct(c *gin.Context) {
	var product models.Product
	if err := s.DB.Where("slug = ? AND is_active = ?", c.Param("slug"), true).First(&product).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			s.fail(c, 404, "not_found", "Produk tidak ditemukan.")
			return
		}
		s.fail(c, 500, "database_error", "Gagal memuat produk.")
		return
	}
	s.ok(c, product)
}

type productInput struct {
	Slug           string               `json:"slug"`
	Name           string               `json:"name" binding:"required"`
	Category       string               `json:"category" binding:"required,oneof=minus fashion"`
	Description    string               `json:"description"`
	Price          int64                `json:"price" binding:"required,gt=0"`
	CompareAtPrice *int64               `json:"compareAtPrice"`
	Image          string               `json:"image"`
	Images         []string             `json:"images"`
	Colors         []models.ColorOption `json:"colors"`
	Gender         []string             `json:"gender"`
	FaceTypes      []string             `json:"faceTypes"`
	Frame          string               `json:"frame"`
	Stock          int                  `json:"stock" binding:"gte=0"`
	IsNew          bool                 `json:"isNew"`
	IsActive       *bool                `json:"isActive"`
}

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	re := regexp.MustCompile(`[^a-z0-9]+`)
	value = re.ReplaceAllString(value, "-")
	return strings.Trim(value, "-")
}
func (s *Server) CreateProduct(c *gin.Context) {
	var input productInput
	if err := c.ShouldBindJSON(&input); err != nil {
		s.fail(c, 400, "invalid_request", "Data produk tidak valid.")
		return
	}
	slug := input.Slug
	if slug == "" {
		slug = slugify(input.Name)
	}
	active := true
	if input.IsActive != nil {
		active = *input.IsActive
	}
	var frameCategory models.Category
	if err := s.DB.Where("name = ?", "Frame").First(&frameCategory).Error; err != nil {
		s.fail(c, 500, "category_unavailable", "Kategori frame belum tersedia di database.")
		return
	}
	product := models.Product{ID: ids.New(), CategoryID: frameCategory.ID, Slug: slug, Name: strings.TrimSpace(input.Name), Category: input.Category, Description: input.Description, Price: input.Price, CompareAtPrice: input.CompareAtPrice, Image: input.Image, Images: input.Images, Colors: input.Colors, Gender: input.Gender, FaceTypes: input.FaceTypes, Frame: input.Frame, Stock: input.Stock, IsNew: input.IsNew, IsActive: active}
	if err := s.DB.Create(&product).Error; err != nil {
		s.fail(c, 409, "product_conflict", "Produk atau slug sudah ada.")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": product})
}
func (s *Server) UpdateProduct(c *gin.Context) {
	var input productInput
	if err := c.ShouldBindJSON(&input); err != nil {
		s.fail(c, 400, "invalid_request", "Data produk tidak valid.")
		return
	}
	var product models.Product
	if err := s.DB.First(&product, "id = ?", c.Param("id")).Error; err != nil {
		s.fail(c, 404, "not_found", "Produk tidak ditemukan.")
		return
	}
	product.Name = strings.TrimSpace(input.Name)
	product.Slug = input.Slug
	if product.Slug == "" {
		product.Slug = slugify(product.Name)
	}
	product.Category = input.Category
	product.Description = input.Description
	product.Price = input.Price
	product.CompareAtPrice = input.CompareAtPrice
	product.Image = input.Image
	product.Images = input.Images
	product.Colors = input.Colors
	product.Gender = input.Gender
	product.FaceTypes = input.FaceTypes
	product.Frame = input.Frame
	product.Stock = input.Stock
	product.IsNew = input.IsNew
	if input.IsActive != nil {
		product.IsActive = *input.IsActive
	}
	if err := s.DB.Save(&product).Error; err != nil {
		s.fail(c, 409, "product_conflict", "Produk tidak dapat diperbarui.")
		return
	}
	s.ok(c, product)
}
func (s *Server) DeleteProduct(c *gin.Context) {
	result := s.DB.Model(&models.Product{}).Where("id = ?", c.Param("id")).Update("is_active", false)
	if result.Error != nil {
		s.fail(c, 500, "database_error", "Produk tidak dapat dinonaktifkan.")
		return
	}
	if result.RowsAffected == 0 {
		s.fail(c, 404, "not_found", "Produk tidak ditemukan.")
		return
	}
	s.ok(c, gin.H{"message": "Produk dinonaktifkan."})
}

func (s *Server) DeleteProductPermanently(c *gin.Context) {
	var product models.Product
	if err := s.DB.First(&product, "id = ?", c.Param("id")).Error; err != nil {
		s.fail(c, 404, "not_found", "Produk tidak ditemukan.")
		return
	}
	var orderItemCount int64
	if err := s.DB.Model(&models.OrderItem{}).Where("product_id = ?", product.ID).Count(&orderItemCount).Error; err != nil {
		s.fail(c, 500, "database_error", "Riwayat pesanan produk tidak dapat diperiksa.")
		return
	}
	if orderItemCount > 0 {
		s.fail(c, 409, "product_in_use", "Produk sudah tercatat di riwayat pesanan. Nonaktifkan produk jika ingin menyembunyikannya.")
		return
	}
	if err := s.DB.Unscoped().Delete(&product).Error; err != nil {
		s.fail(c, 409, "delete_failed", "Produk tidak dapat dihapus.")
		return
	}
	s.ok(c, gin.H{"message": "Produk dihapus permanen."})
}

func (s *Server) SetProductActive(c *gin.Context) {
	var input struct {
		IsActive *bool `json:"isActive" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil || input.IsActive == nil {
		s.fail(c, 400, "invalid_request", "Status aktif produk tidak valid.")
		return
	}
	var product models.Product
	if err := s.DB.First(&product, "id = ?", c.Param("id")).Error; err != nil {
		s.fail(c, 404, "not_found", "Produk tidak ditemukan.")
		return
	}
	if err := s.DB.Model(&product).Update("is_active", *input.IsActive).Error; err != nil {
		s.fail(c, 500, "database_error", "Status produk tidak dapat diperbarui.")
		return
	}
	s.ok(c, gin.H{"productId": product.ID, "isActive": *input.IsActive})
}

type stockInput struct {
	Delta int `json:"delta"`
}

func (s *Server) AdjustStock(c *gin.Context) {
	var input stockInput
	if err := c.ShouldBindJSON(&input); err != nil || input.Delta == 0 {
		s.fail(c, 400, "invalid_request", "Delta stok harus berupa angka selain nol.")
		return
	}
	result := s.DB.Model(&models.Product{}).Where("id = ? AND stock + ? >= 0", c.Param("id"), input.Delta).Update("stock", gorm.Expr("stock + ?", input.Delta))
	if result.Error != nil {
		s.fail(c, 500, "database_error", "Stok tidak dapat diperbarui.")
		return
	}
	if result.RowsAffected == 0 {
		s.fail(c, 409, "insufficient_stock", "Produk tidak ditemukan atau stok tidak boleh negatif.")
		return
	}
	var product models.Product
	_ = s.DB.First(&product, "id = ?", c.Param("id")).Error
	s.ok(c, gin.H{"productId": product.ID, "stock": product.Stock})
}
