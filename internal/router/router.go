package router

import (
	"github.com/gin-gonic/gin"
	"github.com/refa-optical/backend/internal/config"
	"github.com/refa-optical/backend/internal/handlers"
	"github.com/refa-optical/backend/internal/middleware"
)

func New(cfg config.Config, api *handlers.Server) *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery(), middleware.CORS(cfg.AllowedOrigins))
	r.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
	v1 := r.Group("/api/v1")
	authRoutes := v1.Group("/auth")
	authRoutes.POST("/register", api.Register)
	authRoutes.POST("/login", api.Login)
	authRoutes.POST("/refresh", api.Refresh)
	authRoutes.POST("/logout", api.Logout)
	authRoutes.GET("/me", middleware.RequireAuth(cfg.JWTSecret), api.Me)
	v1.GET("/products", api.ListProducts)
	v1.GET("/products/:slug", api.GetProduct)
	v1.GET("/lenses", api.ListLenses)
	protected := v1.Group("")
	protected.Use(middleware.RequireAuth(cfg.JWTSecret))
	protected.POST("/orders", api.CreateOrder)
	protected.GET("/orders", api.ListMyOrders)
	protected.GET("/orders/:id", api.GetMyOrder)
	protected.POST("/orders/:id/prescription", api.UploadPrescription)
	protected.POST("/orders/:id/payments", api.CreatePayment)
	v1.POST("/payments/midtrans/notification", api.MidtransNotification)
	admin := v1.Group("/admin")
	admin.Use(middleware.RequireAuth(cfg.JWTSecret), middleware.AdminOnly())
	admin.GET("/products", api.ListProducts)
	admin.POST("/products", api.CreateProduct)
	admin.PUT("/products/:id", api.UpdateProduct)
	admin.DELETE("/products/:id", api.DeleteProduct)
	admin.DELETE("/products/:id/permanent", api.DeleteProductPermanently)
	admin.PATCH("/products/:id/active", api.SetProductActive)
	admin.PATCH("/products/:id/stock", api.AdjustStock)
	admin.GET("/lenses", api.ListAdminLenses)
	admin.POST("/lenses", api.CreateLens)
	admin.PUT("/lenses/:id", api.UpdateLens)
	admin.DELETE("/lenses/:id", api.DeleteLens)
	admin.DELETE("/lenses/:id/permanent", api.DeleteLensPermanently)
	admin.PATCH("/lenses/:id/active", api.SetLensActive)
	admin.PATCH("/lenses/:id/stock", api.AdjustLensStock)
	admin.GET("/orders", api.ListAdminOrders)
	admin.PATCH("/orders/:id", api.UpdateAdminOrder)
	admin.GET("/orders/:id/prescription", api.AdminOrderPrescription)
	return r
}
