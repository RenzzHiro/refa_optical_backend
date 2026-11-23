package models

import "time"

type User struct {
	ID           string    `gorm:"type:char(36);primaryKey" json:"id"`
	Name         string    `gorm:"size:120;not null" json:"name"`
	Email        string    `gorm:"size:190;not null" json:"email"`
	PasswordHash string    `gorm:"size:255;not null" json:"-"`
	Role         string    `gorm:"size:20;not null;default:customer;index" json:"role"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}
type RefreshToken struct {
	ID        string    `gorm:"type:char(36);primaryKey"`
	UserID    string    `gorm:"type:char(36);not null;index"`
	TokenHash string    `gorm:"size:64;not null"`
	ExpiresAt time.Time `gorm:"not null;index"`
	RevokedAt *time.Time
	CreatedAt time.Time
}
type ColorOption struct {
	Name string `json:"name"`
	Hex  string `json:"hex"`
}
type Category struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Name      string    `gorm:"size:255;not null" json:"name"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
type Product struct {
	ID             string        `gorm:"type:char(36);primaryKey" json:"id"`
	CategoryID     uint          `gorm:"not null;index" json:"categoryId"`
	Slug           string        `gorm:"size:180;not null" json:"slug"`
	Name           string        `gorm:"size:180;not null;index" json:"name"`
	Category       string        `gorm:"size:20;not null;index" json:"category"`
	Description    string        `gorm:"type:text" json:"description"`
	Price          int64         `gorm:"not null" json:"price"`
	CompareAtPrice *int64        `json:"compareAtPrice,omitempty"`
	Image          string        `gorm:"size:1000" json:"image"`
	Images         []string      `gorm:"serializer:json;type:json" json:"images"`
	Colors         []ColorOption `gorm:"serializer:json;type:json" json:"colors"`
	Gender         []string      `gorm:"serializer:json;type:json" json:"gender"`
	FaceTypes      []string      `gorm:"serializer:json;type:json" json:"faceTypes"`
	Frame          string        `gorm:"size:40;index" json:"frame"`
	Stock          int           `gorm:"not null;default:0" json:"stock"`
	IsNew          bool          `gorm:"not null;default:false" json:"isNew"`
	IsActive       bool          `gorm:"not null;default:true;index" json:"isActive"`
	CreatedAt      time.Time     `json:"createdAt"`
	UpdatedAt      time.Time     `json:"updatedAt"`
}
type LensOption struct {
	ID                  string    `gorm:"type:char(36);primaryKey" json:"id"`
	Name                string    `gorm:"size:120;not null" json:"name"`
	Type                string    `gorm:"size:32;not null;index" json:"type"`
	Description         string    `gorm:"type:text" json:"description"`
	Price               int64     `gorm:"not null;default:0" json:"price"`
	Stock               int       `gorm:"not null;default:0" json:"stock"`
	RequiresPrescription bool     `gorm:"not null;default:false" json:"requiresPrescription"`
	IsActive            bool      `gorm:"not null;default:true;index" json:"isActive"`
	CreatedAt           time.Time `json:"createdAt"`
	UpdatedAt           time.Time `json:"updatedAt"`
}
type CustomerAddress struct {
	Name       string `json:"name"`
	Phone      string `json:"phone"`
	Address    string `json:"address"`
	City       string `json:"city"`
	PostalCode string `json:"postalCode"`
}
type Order struct {
	ID                   string          `gorm:"type:char(36);primaryKey" json:"id"`
	Number               string          `gorm:"size:32;not null" json:"number"`
	// Legacy fields are retained for databases created from the earlier Laravel schema.
	OrderNumber          string          `gorm:"column:order_number;size:255" json:"-"`
	CustomerID           string          `gorm:"type:char(36);not null;index" json:"-"`
	// UserID preserves compatibility with legacy Laravel schemas where
	// orders.user_id remains NOT NULL alongside customer_id.
	UserID               string          `gorm:"column:user_id;type:char(36)" json:"-"`
	CustomerEmail        string          `gorm:"size:190;not null;index" json:"-"`
	Status               string          `gorm:"size:32;not null;index" json:"status"`
	PaymentStatus        string          `gorm:"size:32;not null;index" json:"paymentStatus"`
	Subtotal             int64           `gorm:"not null" json:"subtotal"`
	Shipping             int64           `gorm:"not null;default:0" json:"shipping"`
	Total                int64           `gorm:"not null" json:"total"`
	TotalPrice           int64           `gorm:"column:total_price" json:"-"`
	Address              CustomerAddress `gorm:"serializer:json;type:json" json:"address"`
	RequiresPrescription bool            `gorm:"not null;default:false" json:"requiresPrescription"`
	StockReleased        bool            `gorm:"not null;default:false" json:"-"`
	Items                []OrderItem     `gorm:"foreignKey:OrderID" json:"items"`
	Prescription         *Prescription   `gorm:"foreignKey:OrderID" json:"prescription,omitempty"`
	CreatedAt            time.Time       `json:"createdAt"`
	UpdatedAt            time.Time       `json:"updatedAt"`
}
type OrderItem struct {
	ID        uint   `gorm:"primaryKey" json:"-"`
	OrderID   string `gorm:"type:char(36);not null;index" json:"-"`
	ProductID string `gorm:"type:char(36);not null" json:"productId"`
	LensID    *string `gorm:"type:char(36);index" json:"lensId,omitempty"`
	Name      string `gorm:"size:180;not null" json:"name"`
	LensName  string `gorm:"size:120" json:"lensName,omitempty"`
	LensPrice int64  `gorm:"not null;default:0" json:"lensPrice"`
	Image     string `gorm:"size:1000" json:"image"`
	Quantity  int    `gorm:"not null" json:"quantity"`
	Price     int64  `gorm:"not null" json:"price"`
	ColorName string `gorm:"size:80" json:"colorName"`
	Category  string `gorm:"size:20;not null" json:"category"`
}
type Prescription struct {
	ID           string    `gorm:"type:char(36);primaryKey" json:"id"`
	OrderID      string    `gorm:"type:char(36);not null" json:"-"`
	OriginalName string    `gorm:"size:255;not null" json:"originalName"`
	RightSphere  string    `gorm:"size:12;not null;default:''" json:"rightSphere"`
	RightCylinder string   `gorm:"size:12;not null;default:''" json:"rightCylinder"`
	RightAxis    string    `gorm:"size:12;not null;default:''" json:"rightAxis"`
	LeftSphere   string    `gorm:"size:12;not null;default:''" json:"leftSphere"`
	LeftCylinder string    `gorm:"size:12;not null;default:''" json:"leftCylinder"`
	LeftAxis     string    `gorm:"size:12;not null;default:''" json:"leftAxis"`
	PD           string    `gorm:"size:12;not null;default:''" json:"pd"`
	ObjectKey    string    `gorm:"size:500;not null" json:"-"`
	ContentType  string    `gorm:"size:100;not null" json:"contentType"`
	Size         int64     `gorm:"not null" json:"size"`
	CreatedAt    time.Time `json:"createdAt"`
}
type Payment struct {
	ID             string `gorm:"type:char(36);primaryKey"`
	OrderID        string `gorm:"type:char(36);not null"`
	GatewayOrderID string `gorm:"size:50;not null"`
	SnapToken      string `gorm:"size:500"`
	RedirectURL    string `gorm:"size:1000"`
	TransactionID  string `gorm:"size:100"`
	Status         string `gorm:"size:32;not null;default:pending"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
