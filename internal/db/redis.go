package db

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

var RedisNil = redis.Nil

// RedisClient Redis 缓存封装
type RedisClient struct {
	client *redis.Client
}

func NewRedisClient(addr, password string) *RedisClient {
	rdb := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     password,
		DB:           0,
		PoolSize:     100,
		MinIdleConns: 10,
	})
	return &RedisClient{client: rdb}
}

func (r *RedisClient) Close() error {
	return r.client.Close()
}

func (r *RedisClient) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

const (
	SessionTTL  = 5 * time.Minute
	OnlineTTL   = 30 * time.Minute
	PositionTTL = 30 * time.Minute

	KeySession  = "session:"  // session:<token> → account_id
	KeyOnline   = "online:"   // online:<uid> → scene_addr
	KeyPosition = "pos:"      // pos:<uid> → hash {x, y, map_id}
)

// SetSession 存储 Session 绑定关系
func (r *RedisClient) SetSession(ctx context.Context, token string, accountID int64) error {
	return r.client.Set(ctx, KeySession+token, accountID, SessionTTL).Err()
}

// GetSession 获取 Session 对应的账号 ID
func (r *RedisClient) GetSession(ctx context.Context, token string) (int64, error) {
	return r.client.Get(ctx, KeySession+token).Int64()
}

// DelSession 删除 Session
func (r *RedisClient) DelSession(ctx context.Context, token string) error {
	return r.client.Del(ctx, KeySession+token).Err()
}

// SetOnline 设置在线状态
func (r *RedisClient) SetOnline(ctx context.Context, roleID int64, sceneAddr string) error {
	return r.client.Set(ctx, KeyOnline+formatInt64(roleID), sceneAddr, OnlineTTL).Err()
}

// GetOnline 查询在线状态
func (r *RedisClient) GetOnline(ctx context.Context, roleID int64) (string, error) {
	return r.client.Get(ctx, KeyOnline+formatInt64(roleID)).Result()
}

// DelOnline 清除在线状态
func (r *RedisClient) DelOnline(ctx context.Context, roleID int64) error {
	return r.client.Del(ctx, KeyOnline+formatInt64(roleID)).Err()
}

// SetPosition 缓存角色位置
func (r *RedisClient) SetPosition(ctx context.Context, roleID int64, x, y float64, mapID int) error {
	return r.client.HSet(ctx, KeyPosition+formatInt64(roleID), map[string]interface{}{
		"x":      x,
		"y":      y,
		"map_id": mapID,
		"time":   time.Now().Unix(),
	}).Err()
}

// GetPosition 获取角色位置缓存
func (r *RedisClient) GetPosition(ctx context.Context, roleID int64) (x, y float64, mapID int, err error) {
	result, err := r.client.HGetAll(ctx, KeyPosition+formatInt64(roleID)).Result()
	if err != nil {
		return 0, 0, 0, err
	}
	if len(result) == 0 {
		return 0, 0, 0, RedisNil
	}
	x = parseFloat(result["x"])
	y = parseFloat(result["y"])
	mapID = parseInt(result["map_id"])
	return
}

// DelPosition 删除位置缓存
func (r *RedisClient) DelPosition(ctx context.Context, roleID int64) error {
	return r.client.Del(ctx, KeyPosition+formatInt64(roleID)).Err()
}

// ========== 内部辅助 ==========

func formatInt64(v int64) string {
	buf := make([]byte, 0, 20)
	return string(appendInt64(buf, v))
}

func appendInt64(buf []byte, v int64) []byte {
	if v == 0 {
		return append(buf, '0')
	}
	if v < 0 {
		buf = append(buf, '-')
		v = -v
	}
	var tmp [20]byte
	i := len(tmp)
	for v > 0 {
		i--
		tmp[i] = byte('0' + v%10)
		v /= 10
	}
	return append(buf, tmp[i:]...)
}

func parseFloat(s string) float64 {
	if s == "" {
		return 0
	}
	var neg bool
	i := 0
	if s[i] == '-' {
		neg = true
		i++
	}
	var intPart, fracPart, fracDiv float64
	for i < len(s) && s[i] != '.' {
		intPart = intPart*10 + float64(s[i]-'0')
		i++
	}
	if i < len(s) && s[i] == '.' {
		i++
		fracDiv = 1
		for i < len(s) {
			fracPart = fracPart*10 + float64(s[i]-'0')
			fracDiv *= 10
			i++
		}
	}
	result := intPart + fracPart/fracDiv
	if neg {
		return -result
	}
	return result
}

func parseInt(s string) int {
	if s == "" {
		return 0
	}
	var neg bool
	i := 0
	if s[i] == '-' {
		neg = true
		i++
	}
	var n int
	for i < len(s) {
		n = n*10 + int(s[i]-'0')
		i++
	}
	if neg {
		return -n
	}
	return n
}
