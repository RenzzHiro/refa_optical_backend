package database

import (
	"github.com/refa-optical/backend/internal/models"
	"gorm.io/gorm"
)

func SeedCatalog(db *gorm.DB) error {
	for _, name := range []string{"Lensa", "Frame"} {
		if err := db.Where("name = ?", name).FirstOrCreate(&models.Category{}, models.Category{Name: name}).Error; err != nil {
			return err
		}
	}
	var frameCategory models.Category
	if err := db.Where("name = ?", "Frame").First(&frameCategory).Error; err != nil {
		return err
	}
	var count int64
	if err := db.Model(&models.Product{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	products := []models.Product{
		{ID: "demo-aika-round", CategoryID: frameCategory.ID, Slug: "aika-round-01", Name: "Aika Round 01", Category: "minus", Description: "Frame bulat ringan untuk tampilan sehari-hari.", Price: 424150, CompareAtPrice: ptr(499000), Image: "https://images.unsplash.com/photo-1574258495973-f010dfbb5371?auto=format&fit=crop&w=1000&q=85", Images: []string{"https://images.unsplash.com/photo-1574258495973-f010dfbb5371?auto=format&fit=crop&w=1000&q=85"}, Colors: []models.ColorOption{{Name: "Onyx", Hex: "#171717"}, {Name: "Champagne", Hex: "#d6c6a5"}}, Gender: []string{"Wanita", "Unisex"}, FaceTypes: []string{"Kotak", "Oval"}, Frame: "Bulat", Stock: 12, IsNew: true, IsActive: true},
		{ID: "demo-argo-classic", CategoryID: frameCategory.ID, Slug: "argo-classic", Name: "Argo Classic", Category: "minus", Description: "Frame persegi klasik untuk pemakaian harian.", Price: 389000, Image: "https://images.unsplash.com/photo-1509695507497-903c140c43b0?auto=format&fit=crop&w=1000&q=85", Images: []string{"https://images.unsplash.com/photo-1509695507497-903c140c43b0?auto=format&fit=crop&w=1000&q=85"}, Colors: []models.ColorOption{{Name: "Black", Hex: "#171717"}}, Gender: []string{"Pria", "Unisex"}, FaceTypes: []string{"Bulat", "Oval"}, Frame: "Kotak", Stock: 9, IsActive: true},
		{ID: "demo-aira-cat-eye", CategoryID: frameCategory.ID, Slug: "aira-cat-eye", Name: "Aira Cat Eye", Category: "fashion", Description: "Siluet cat eye dengan garis tegas.", Price: 254150, CompareAtPrice: ptr(299000), Image: "https://images.unsplash.com/photo-1508296695146-257a814070b4?auto=format&fit=crop&w=1000&q=85", Images: []string{"https://images.unsplash.com/photo-1508296695146-257a814070b4?auto=format&fit=crop&w=1000&q=85"}, Colors: []models.ColorOption{{Name: "Black", Hex: "#171717"}, {Name: "Amber", Hex: "#9e541a"}}, Gender: []string{"Wanita"}, FaceTypes: []string{"Bulat", "Oval"}, Frame: "Cat eye", Stock: 17, IsActive: true},
	}
	return db.Create(&products).Error
}
func ptr(value int64) *int64 { return &value }
