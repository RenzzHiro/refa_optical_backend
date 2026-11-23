package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/refa-optical/backend/internal/ids"
	"github.com/refa-optical/backend/internal/models"
	"github.com/refa-optical/backend/internal/payment"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"math/big"
	"net/http"
	"strings"
	"time"
)

func (s *Server) CreatePayment(c *gin.Context) {
	var order models.Order
	if err := s.DB.Preload("Items").Preload("Prescription").Where("id = ? AND customer_id = ?", c.Param("id"), s.userID(c)).First(&order).Error; err != nil {
		s.fail(c, 404, "not_found", "Pesanan tidak ditemukan.")
		return
	}
	if order.PaymentStatus == "paid" {
		s.fail(c, 409, "already_paid", "Pesanan sudah dibayar.")
		return
	}
	if order.RequiresPrescription && order.Prescription == nil {
		s.fail(c, 409, "prescription_required", "Unggah foto resep sebelum memulai pembayaran.")
		return
	}
	if s.Config.MidtransServerKey == "" {
		s.fail(c, 503, "payment_unconfigured", "MIDTRANS_SERVER_KEY belum dikonfigurasi.")
		return
	}
	var existing models.Payment
	if err := s.DB.Where("order_id = ?", order.ID).First(&existing).Error; err == nil && existing.SnapToken != "" {
		s.ok(c, gin.H{"orderId": order.ID, "snapToken": existing.SnapToken, "redirectUrl": existing.RedirectURL, "paymentStatus": order.PaymentStatus})
		return
	}
	var user models.User
	if err := s.DB.First(&user, "id = ?", s.userID(c)).Error; err != nil {
		s.fail(c, 500, "database_error", "Data pelanggan tidak dapat dimuat.")
		return
	}
	result, err := s.Gateway.CreateSnap(c.Request.Context(), order, user, s.Config.FrontendURL)
	if err != nil {
		s.fail(c, 502, "payment_gateway_error", "Payment gateway tidak dapat membuat transaksi.")
		return
	}
	record := models.Payment{ID: ids.New(), OrderID: order.ID, GatewayOrderID: order.Number, SnapToken: result.Token, RedirectURL: result.RedirectURL, Status: "pending"}
	if existing.ID != "" {
		record.ID = existing.ID
		if err = s.DB.Model(&existing).Updates(map[string]any{"snap_token": record.SnapToken, "redirect_url": record.RedirectURL, "status": "pending"}).Error; err != nil {
			s.fail(c, 500, "database_error", "Transaksi pembayaran tidak dapat disimpan.")
			return
		}
	} else if err = s.DB.Create(&record).Error; err != nil {
		s.fail(c, 500, "database_error", "Transaksi pembayaran tidak dapat disimpan.")
		return
	}
	s.ok(c, gin.H{"orderId": order.ID, "snapToken": result.Token, "redirectUrl": result.RedirectURL, "paymentStatus": "pending"})
}

type midtransNotification struct {
	OrderID           string `json:"order_id"`
	StatusCode        string `json:"status_code"`
	GrossAmount       string `json:"gross_amount"`
	SignatureKey      string `json:"signature_key"`
	TransactionStatus string `json:"transaction_status"`
	FraudStatus       string `json:"fraud_status"`
	TransactionID     string `json:"transaction_id"`
}

func (s *Server) MidtransNotification(c *gin.Context) {
	var notification midtransNotification
	if err := c.ShouldBindJSON(&notification); err != nil {
		s.fail(c, 400, "invalid_notification", "Payload notifikasi tidak valid.")
		return
	}
	if s.Config.MidtransServerKey == "" {
		s.fail(c, 503, "payment_unconfigured", "MIDTRANS_SERVER_KEY belum dikonfigurasi.")
		return
	}
	verifiedSignature := payment.VerifyNotification(notification.OrderID, notification.StatusCode, notification.GrossAmount, notification.SignatureKey, s.Config.MidtransServerKey)
	status, statusErr := s.Gateway.GetStatus(c.Request.Context(), notification.OrderID)
	if statusErr != nil {
		if !verifiedSignature {
			s.fail(c, 401, "invalid_signature", "Notifikasi tidak dapat diverifikasi oleh Midtrans.")
			return
		}
		s.fail(c, 502, "payment_status_unavailable", "Status pembayaran belum dapat diverifikasi.")
		return
	}
	if status.OrderID != notification.OrderID {
		s.fail(c, 502, "payment_status_mismatch", "Status pembayaran tidak cocok dengan notifikasi.")
		return
	}
	notification.TransactionStatus = status.TransactionStatus
	notification.FraudStatus = status.FraudStatus
	notification.TransactionID = status.TransactionID
	if status.GrossAmount != "" {
		notification.GrossAmount = status.GrossAmount
	}
	var record models.Payment
	if err := s.DB.Where("gateway_order_id = ?", notification.OrderID).First(&record).Error; err != nil {
		s.fail(c, 404, "payment_not_found", "Transaksi pembayaran tidak ditemukan.")
		return
	}
	var total models.Order
	if err := s.DB.Select("total").First(&total, "id = ?", record.OrderID).Error; err != nil {
		s.fail(c, 404, "order_not_found", "Pesanan pembayaran tidak ditemukan.")
		return
	}
	received, ok := new(big.Rat).SetString(notification.GrossAmount)
	expected := new(big.Rat).SetInt64(total.Total)
	if !ok || received.Cmp(expected) != 0 {
		s.fail(c, 400, "amount_mismatch", "Nominal notifikasi pembayaran tidak sesuai pesanan.")
		return
	}
	nextPayment := record.Status
	nextOrder := ""
	release := false
	switch strings.ToLower(notification.TransactionStatus) {
	case "settlement":
		if notification.FraudStatus == "" || strings.EqualFold(notification.FraudStatus, "accept") {
			nextPayment = "paid"
			nextOrder = "processing"
		} else {
			nextPayment = "failed"
			nextOrder = "cancelled"
			release = true
		}
	case "capture":
		if notification.FraudStatus == "" || strings.EqualFold(notification.FraudStatus, "accept") {
			nextPayment = "paid"
			nextOrder = "processing"
		} else {
			nextPayment = "failed"
			nextOrder = "cancelled"
			release = true
		}
	case "pending":
		nextPayment = "pending"
	case "deny":
		nextPayment = "failed"
		nextOrder = "cancelled"
		release = true
	case "expire":
		nextPayment = "expired"
		nextOrder = "cancelled"
		release = true
	case "cancel":
		nextPayment = "cancelled"
		nextOrder = "cancelled"
		release = true
	default:
		c.JSON(http.StatusOK, gin.H{"status": "ignored"})
		return
	}
	effectivePayment := nextPayment
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		var order models.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Preload("Items").First(&order, "id = ?", record.OrderID).Error; err != nil {
			return err
		}
		if order.PaymentStatus == "paid" && nextPayment != "paid" {
			effectivePayment = "paid"
		}
		if order.PaymentStatus != "paid" || nextPayment == "paid" {
			order.PaymentStatus = effectivePayment
			if nextOrder != "" {
				order.Status = nextOrder
			}
		}
		if release && order.PaymentStatus != "paid" && !order.StockReleased {
			for _, item := range order.Items {
				if err := tx.Model(&models.Product{}).Where("id = ?", item.ProductID).Update("stock", gorm.Expr("stock + ?", item.Quantity)).Error; err != nil {
					return err
				}
				if item.LensID != nil && *item.LensID != "" {
					if err := tx.Model(&models.LensOption{}).Where("id = ?", *item.LensID).Update("stock", gorm.Expr("stock + ?", item.Quantity)).Error; err != nil { return err }
				}
			}
			order.StockReleased = true
		}
		if err := tx.Save(&order).Error; err != nil {
			return err
		}
		return tx.Model(&models.Payment{}).Where("id = ?", record.ID).Updates(map[string]any{"status": effectivePayment, "transaction_id": notification.TransactionID, "updated_at": time.Now()}).Error
	})
	if err != nil {
		s.fail(c, 500, "notification_processing_error", "Notifikasi pembayaran belum dapat diproses.")
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
