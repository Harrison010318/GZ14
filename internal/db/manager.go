package db

import (
	"context"
	"log"
	"math"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Manager DB 管理，封装 MySQL + Redis 操作
type Manager struct {
	mysql *gorm.DB
	redis *RedisClient
}

func NewManager(dsn, redisAddr, redisPass string) (*Manager, error) {
	// MySQL
	gormDB, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, err
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)

	// 自动建表
	if err := AutoMigrate(gormDB); err != nil {
		return nil, err
	}

	// Redis
	rdb := NewRedisClient(redisAddr, redisPass)

	return &Manager{
		mysql: gormDB,
		redis: rdb,
	}, nil
}

func (m *Manager) Close() {
	if m.redis != nil {
		m.redis.Close()
	}
}

// ========== 账号 ==========

func (m *Manager) CreateAccount(ctx context.Context, username, password string) (*Account, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 8)
	if err != nil {
		return nil, err
	}
	acc := &Account{
		Username: username,
		Password: string(hash),
	}
	if err := m.mysql.WithContext(ctx).Create(acc).Error; err != nil {
		return nil, err
	}
	return acc, nil
}

func (m *Manager) GetAccountByUsername(ctx context.Context, username string) (*Account, error) {
	var acc Account
	err := m.mysql.WithContext(ctx).Where("username = ?", username).First(&acc).Error
	if err != nil {
		return nil, err
	}
	return &acc, nil
}

func (m *Manager) VerifyPassword(hashed, plain string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashed), []byte(plain))
	return err == nil
}

// ========== 角色 ==========

func (m *Manager) CreateRole(ctx context.Context, accountID int64, name string) (*Role, error) {
	// 检查角色数量上限
	var count int64
	m.mysql.WithContext(ctx).Model(&Role{}).Where("account_id = ?", accountID).Count(&count)
	if count >= 3 {
		return nil, ErrRoleFull
	}

	role := &Role{
		AccountID: accountID,
		Name:      name,
		MapID:     1,
		PosX:      100,
		PosY:      100,
	}
	if err := m.mysql.WithContext(ctx).Create(role).Error; err != nil {
		return nil, err
	}
	return role, nil
}

func (m *Manager) GetRolesByAccount(ctx context.Context, accountID int64) ([]Role, error) {
	var roles []Role
	err := m.mysql.WithContext(ctx).Where("account_id = ?", accountID).Find(&roles).Error
	return roles, err
}

func (m *Manager) GetRoleByID(ctx context.Context, roleID int64) (*Role, error) {
	var role Role
	err := m.mysql.WithContext(ctx).First(&role, roleID).Error
	if err != nil {
		return nil, err
	}
	return &role, nil
}

func (m *Manager) IsRoleNameExist(ctx context.Context, name string) (bool, error) {
	var count int64
	err := m.mysql.WithContext(ctx).Model(&Role{}).Where("name = ?", name).Count(&count).Error
	return count > 0, err
}

func (m *Manager) UpdateRolePosition(ctx context.Context, roleID int64, x, y float64, mapID int) error {
	if math.IsNaN(x) || math.IsNaN(y) {
		return nil // 忽略 NaN 位置，防止数据库写入失败
	}
	return m.mysql.WithContext(ctx).Model(&Role{}).Where("id = ?", roleID).Updates(map[string]interface{}{
		"pos_x": x,
		"pos_y": y,
		"map_id": mapID,
	}).Error
}

// ========== Session（Redis）==========

func (m *Manager) SetSession(ctx context.Context, token string, accountID int64) error {
	return m.redis.SetSession(ctx, token, accountID)
}

func (m *Manager) GetSession(ctx context.Context, token string) (int64, error) {
	return m.redis.GetSession(ctx, token)
}

func (m *Manager) DelSession(ctx context.Context, token string) error {
	return m.redis.DelSession(ctx, token)
}

// ========== 在线状态（Redis）==========

func (m *Manager) SetOnline(ctx context.Context, roleID int64, sceneAddr string) error {
	return m.redis.SetOnline(ctx, roleID, sceneAddr)
}

func (m *Manager) GetOnline(ctx context.Context, roleID int64) (string, error) {
	return m.redis.GetOnline(ctx, roleID)
}

func (m *Manager) DelOnline(ctx context.Context, roleID int64) error {
	return m.redis.DelOnline(ctx, roleID)
}

// ========== 位置缓存（Redis）==========

func (m *Manager) SetPosition(ctx context.Context, roleID int64, x, y float64, mapID int) error {
	return m.redis.SetPosition(ctx, roleID, x, y, mapID)
}

func (m *Manager) GetPosition(ctx context.Context, roleID int64) (x, y float64, mapID int, err error) {
	return m.redis.GetPosition(ctx, roleID)
}

func (m *Manager) DelPosition(ctx context.Context, roleID int64) error {
	return m.redis.DelPosition(ctx, roleID)
}

// ========== 位置同步（写回策略）==========

// SyncPosition 将缓存中的位置批量写入 MySQL
func (m *Manager) SyncPosition(ctx context.Context, roleIDs []int64) {
	for _, rid := range roleIDs {
		x, y, mapID, err := m.GetPosition(ctx, rid)
		if err != nil {
			continue
		}
		if err := m.UpdateRolePosition(ctx, rid, x, y, mapID); err != nil {
			log.Printf("[DB] sync position failed for role %d: %v", rid, err)
		}
	}
}
