package handlers

import (
	"fmt"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/refa-optical/backend/internal/ids"
	"github.com/refa-optical/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type orderCreateInput struct {
	Items []struct {
		ProductID string `json:"productId" binding:"required"`
		LensID    string `json:"lensId" binding:"required"`
		Quantity  int    `json:"quantity" binding:"required,min=1,max=20"`
		ColorName string `json:"colorName"`
	} `json:"items" binding:"required,min=1"`
	Address models.CustomerAddress `json:"address"`
}

func (s *Server) CreateOrder(c *gin.Context) {
	var input orderCreateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		s.fail(c, 400, "invalid_request", "Item pesanan dan alamat penerima tidak valid.")
		return
	}
	address := input.Address
	if strings.TrimSpace(address.Name) == "" || strings.TrimSpace(address.Phone) == "" || strings.TrimSpace(address.Address) == "" || strings.TrimSpace(address.City) == "" || strings.TrimSpace(address.PostalCode) == "" {
		s.fail(c, 400, "invalid_address", "Nama, telepon, alamat, kota, dan kode pos wajib diisi.")
		return
	}
	userID := s.userID(c)
	var order models.Order
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		var user models.User
		if err := tx.First(&user, "id = ?", userID).Error; err != nil {
			return err
		}
		order = models.Order{ID: ids.New(), Number: "RO-" + strings.ToUpper(strings.ReplaceAll(ids.New(), "-", ""))[:12], CustomerID: user.ID, UserID: user.ID, CustomerEmail: user.Email, Status: "awaiting_payment", PaymentStatus: "pending", Address: input.Address, Shipping: 0}
		for _, line := range input.Items {
			var product models.Product
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND is_active = ?", line.ProductID, true).First(&product).Error; err != nil {
				return fmt.Errorf("product unavailable: %s", line.ProductID)
			}
			if product.Stock < line.Quantity {
				return fmt.Errorf("insufficient stock: %s", product.ID)
			}
			var lens models.LensOption
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND is_active = ?", line.LensID, true).First(&lens).Error; err != nil {
				return fmt.Errorf("lens unavailable: %s", line.LensID)
			}
			if lens.Stock < line.Quantity {
				return fmt.Errorf("insufficient lens stock: %s", lens.ID)
			}
			product.Stock -= line.Quantity
			if err := tx.Model(&product).Update("stock", product.Stock).Error; err != nil {
				return err
			}
			lens.Stock -= line.Quantity
			if err := tx.Model(&lens).Update("stock", lens.Stock).Error; err != nil {
				return err
			}
			linePrice := product.Price + lens.Price
			order.Subtotal += linePrice * int64(line.Quantity)
			if product.Category == "minus" || lens.RequiresPrescription {
				order.RequiresPrescription = true
			}
			lensID := lens.ID
			order.Items = append(order.Items, models.OrderItem{ProductID: product.ID, LensID: &lensID, Name: product.Name, LensName: lens.Name, LensPrice: lens.Price, Image: product.Image, Quantity: line.Quantity, Price: linePrice, ColorName: line.ColorName, Category: product.Category})
		}
		order.Total = order.Subtotal + order.Shipping
		order.OrderNumber = order.Number
		order.TotalPrice = order.Total
		return tx.Create(&order).Error
	})
	if err != nil {
		if strings.Contains(err.Error(), "insufficient stock") {
			s.fail(c, 409, "insufficient_stock", "Stok produk tidak mencukupi.")
			return
		}
		if strings.Contains(err.Error(), "product unavailable") {
			s.fail(c, 409, "product_unavailable", "Produk tidak tersedia.")
			return
		}
		if strings.Contains(err.Error(), "insufficient lens stock") {
			s.fail(c, 409, "insufficient_lens_stock", "Stok lensa tidak mencukupi.")
			return
		}
		if strings.Contains(err.Error(), "lens unavailable") {
			s.fail(c, 409, "lens_unavailable", "Pilihan lensa sudah tidak tersedia.")
			return
		}
		s.fail(c, 500, "order_error", "Pesanan tidak dapat dibuat.")
		return
	}
	s.DB.Preload("Items").First(&order, "id = ?", order.ID)
	c.JSON(http.StatusCreated, gin.H{"data": order})
}
func (s *Server) ListMyOrders(c *gin.Context) {
	var orders []models.Order
	if err := s.DB.Preload("Items").Where("customer_id = ?", s.userID(c)).Order("created_at DESC").Find(&orders).Error; err != nil {
		s.fail(c, 500, "database_error", "Riwayat pesanan tidak dapat dimuat.")
		return
	}
	s.ok(c, orders)
}
func (s *Server) GetMyOrder(c *gin.Context) {
	var order models.Order
	if err := s.DB.Preload("Items").Preload("Prescription").Where("id = ? AND customer_id = ?", c.Param("id"), s.userID(c)).First(&order).Error; err != nil {
		s.fail(c, 404, "not_found", "Pesanan tidak ditemukan.")
		return
	}
	s.ok(c, order)
}

type prescriptionDetails struct {
	RightSphere string `json:"rightSphere" binding:"required"`
	RightCylinder string `json:"rightCylinder" binding:"required"`
	RightAxis string `json:"rightAxis" binding:"required"`
	LeftSphere string `json:"leftSphere" binding:"required"`
	LeftCylinder string `json:"leftCylinder" binding:"required"`
	LeftAxis string `json:"leftAxis" binding:"required"`
	PD string `json:"pd" binding:"required"`
}

func validatePrescription(details prescriptionDetails) bool {
	for _, value := range []string{details.RightSphere, details.RightCylinder, details.LeftSphere, details.LeftCylinder} {
		if _, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err != nil { return false }
	}
	pd, err := strconv.ParseFloat(strings.TrimSpace(details.PD), 64)
	if err != nil || pd <= 0 || pd > 90 { return false }
	for _, value := range []string{details.RightAxis, details.LeftAxis} {
		axis, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || axis < 0 || axis > 180 { return false }
	}
	return true
}

func (s *Server) UploadPrescription(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, s.Config.MaxUploadBytes+1024)
	header, err := c.FormFile("file")
	if err != nil {
		s.fail(c, 400, "invalid_file", "File resep wajib dilampirkan.")
		return
	}
	if header.Size > s.Config.MaxUploadBytes {
		s.fail(c, 413, "file_too_large", "Ukuran file melebihi batas 5 MB.")
		return
	}
	file, err := header.Open()
	if err != nil {
		s.fail(c, 400, "invalid_file", "File tidak dapat dibaca.")
		return
	}
	defer file.Close()
	buffer := make([]byte, 512)
	n, _ := file.Read(buffer)
	contentType := http.DetectContentType(buffer[:n])
	exts := map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "application/pdf": ".pdf"}
	ext, ok := exts[contentType]
	if !ok {
		s.fail(c, 415, "unsupported_file", "Format yang diterima: JPG, PNG, atau PDF.")
		return
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		s.fail(c, 400, "invalid_file", "File tidak dapat dibaca.")
		return
	}
	var order models.Order
	if err = s.DB.Preload("Prescription").Where("id = ? AND customer_id = ?", c.Param("id"), s.userID(c)).First(&order).Error; err != nil {
		s.fail(c, 404, "not_found", "Pesanan tidak ditemukan.")
		return
	}
	if !order.RequiresPrescription {
		s.fail(c, 409, "not_required", "Pesanan ini tidak memerlukan resep.")
		return
	}
	var details prescriptionDetails
	if err = json.Unmarshal([]byte(c.PostForm("details")), &details); err != nil || !validatePrescription(details) {
		s.fail(c, 400, "invalid_prescription", "Isi SPH, CYL, axis untuk mata kanan dan kiri, serta PD dengan benar.")
		return
	}
	key := filepath.ToSlash(filepath.Join("prescriptions", order.ID, ids.New()+ext))
	if err = s.Store.Put(c.Request.Context(), key, file, header.Size, contentType); err != nil {
		s.fail(c, 500, "storage_error", "File resep tidak dapat disimpan.")
		return
	}
	record := models.Prescription{ID: ids.New(), OrderID: order.ID, OriginalName: filepath.Base(header.Filename), RightSphere: details.RightSphere, RightCylinder: details.RightCylinder, RightAxis: details.RightAxis, LeftSphere: details.LeftSphere, LeftCylinder: details.LeftCylinder, LeftAxis: details.LeftAxis, PD: details.PD, ObjectKey: key, ContentType: contentType, Size: header.Size, CreatedAt: time.Now()}
	if order.Prescription != nil {
		_ = s.Store.Delete(c.Request.Context(), order.Prescription.ObjectKey)
		if err = s.DB.Delete(&models.Prescription{}, "id = ?", order.Prescription.ID).Error; err != nil {
			s.fail(c, 500, "database_error", "Metadata resep tidak dapat diperbarui.")
			return
		}
	}
	if err = s.DB.Create(&record).Error; err != nil {
		_ = s.Store.Delete(c.Request.Context(), key)
		s.fail(c, 500, "database_error", "Metadata resep tidak dapat disimpan.")
		return
	}
	s.ok(c, gin.H{"id": record.ID, "fileName": record.OriginalName, "contentType": record.ContentType, "size": record.Size})
}
func (s *Server) ListAdminOrders(c *gin.Context) {
	var orders []models.Order
	query := s.DB.Preload("Items").Preload("Prescription")
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	if err := query.Order("created_at DESC").Find(&orders).Error; err != nil {
		s.fail(c, 500, "database_error", "Pesanan tidak dapat dimuat.")
		return
	}
	s.ok(c, orders)
}
func (s *Server) AdminOrderPrescription(c *gin.Context) {
	var order models.Order
	if err := s.DB.Preload("Prescription").First(&order, "id = ?", c.Param("id")).Error; err != nil || order.Prescription == nil {
		s.fail(c, 404, "not_found", "Lampiran resep tidak ditemukan.")
		return
	}
	reader, err := s.Store.Open(c.Request.Context(), order.Prescription.ObjectKey)
	if err != nil {
		s.fail(c, 404, "file_missing", "File resep tidak tersedia.")
		return
	}
	defer reader.Close()
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", order.Prescription.OriginalName))
	c.DataFromReader(http.StatusOK, order.Prescription.Size, order.Prescription.ContentType, reader, nil)
}
func (s *Server) UpdateAdminOrder(c *gin.Context) {
	var input struct {
		Status string `json:"status" binding:"required,oneof=processing completed cancelled`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		s.fail(c, 400, "invalid_request", "Status pesanan tidak valid.")
		return
	}
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		var order models.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Preload("Items").First(&order, "id = ?", c.Param("id")).Error; err != nil {
			return err
		}
		if (input.Status == "processing" || input.Status == "completed") && order.PaymentStatus != "paid" {
			return fmt.Errorf("cannot advance unpaid order")
		}
		if input.Status == "cancelled" && order.PaymentStatus == "paid" {
			return fmt.Errorf("paid order cancellation requires refund")
		}
		if order.Status == "cancelled" && input.Status != "cancelled" {
			return fmt.Errorf("cancelled order cannot be reopened")
		}
		if input.Status == "cancelled" && !order.StockReleased {
			for _, item := range order.Items {
				if err := tx.Model(&models.Product{}).Where("id = ?", item.ProductID).Update("stock", gorm.Expr("stock + ?", item.Quantity)).Error; err != nil {
					return err
				}
				if item.LensID != nil && *item.LensID != "" {
					if err := tx.Model(&models.LensOption{}).Where("id = ?", *item.LensID).Update("stock", gorm.Expr("stock + ?", item.Quantity)).Error; err != nil { return err }
				}
			}
			order.StockReleased = true
			order.PaymentStatus = "cancelled"
		}
		order.Status = input.Status
		return tx.Save(&order).Error
	})
	if err != nil {
		if strings.Contains(err.Error(), "cannot") || strings.Contains(err.Error(), "requires refund") {
			s.fail(c, 409, "invalid_transition", "Transisi status pesanan tidak diperbolehkan.")
			return
		}
		s.fail(c, 404, "not_found", "Pesanan tidak dapat diperbarui.")
		return
	}
	var order models.Order
	s.DB.Preload("Items").First(&order, "id = ?", c.Param("id"))
	s.ok(c, order)
}
