package config

import (
	"os"
)

type Config struct {
	// Lobby 服务
	LobbyAddr  string
	LobbyPort  string

	// Scene 服务
	SceneAddr  string
	ScenePort  string

	// DB 服务
	DBAddr     string
	DBPort     string

	// MySQL
	MySQLDSN   string

	// Redis
	RedisAddr  string
	RedisPass  string

	// 场景参数
	MapWidth   float64
	MapHeight  float64
	GridSize   float64
}

func Load() *Config {
	return &Config{
		LobbyAddr: getEnv("LOBBY_ADDR", "0.0.0.0"),
		LobbyPort: getEnv("LOBBY_PORT", "19001"),
		SceneAddr: getEnv("SCENE_ADDR", "0.0.0.0"),
		ScenePort: getEnv("SCENE_PORT", "19002"),
		DBAddr:    getEnv("DB_ADDR", "0.0.0.0"),
		DBPort:    getEnv("DB_PORT", "19003"),
		MySQLDSN:  getEnv("MYSQL_DSN", "root@tcp(127.0.0.1:3306)/gz14?charset=utf8mb4&parseTime=True"),
		RedisAddr: getEnv("REDIS_ADDR", "127.0.0.1:6379"),
		RedisPass: getEnv("REDIS_PASS", ""),
		MapWidth:  1000.0,
		MapHeight: 1000.0,
		GridSize:  100.0,
	}
}

func (c *Config) LobbyListenAddr() string {
	return c.LobbyAddr + ":" + c.LobbyPort
}

func (c *Config) SceneListenAddr() string {
	return c.SceneAddr + ":" + c.ScenePort
}

func (c *Config) DBListenAddr() string {
	return c.DBAddr + ":" + c.DBPort
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
