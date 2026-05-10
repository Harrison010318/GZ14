package db

import (
	"time"

	"gorm.io/gorm"
)

// Account 账号表
type Account struct {
	ID        int64     `gorm:"primaryKey;autoIncrement"`
	Username  string    `gorm:"uniqueIndex;size:32;not null"`
	Password  string    `gorm:"size:128;not null"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}

// Role 角色表
type Role struct {
	ID        int64     `gorm:"primaryKey;autoIncrement"`
	AccountID int64     `gorm:"index;not null"`
	Name      string    `gorm:"uniqueIndex;size:16;not null"`
	Level     int       `gorm:"default:1"`
	MapID     int       `gorm:"default:1"`
	PosX      float64   `gorm:"default:0"`
	PosY      float64   `gorm:"default:0"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}

// TableName 指定表名
func (Account) TableName() string {
	return "accounts"
}

func (Role) TableName() string {
	return "roles"
}

// AutoMigrate 自动建表
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(&Account{}, &Role{})
}
