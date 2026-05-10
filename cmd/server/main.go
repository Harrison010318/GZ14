package main

import (
	"flag"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"

	"gz14/internal/db"
	"gz14/internal/lobby"
	"gz14/internal/scene"
	"gz14/pkg/config"
)

func main() {
	configPath := flag.String("config", "", "config file path (optional)")
	pprofAddr := flag.String("pprof", "", "pprof listen address (e.g. :6060)")
	flag.Parse()

	cfg := config.Load()
	if *configPath != "" {
		_ = *configPath // 预留配置文件支持
	}

	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("[Server] GZ-14 game backend starting...")

	// 可选启动 pprof
	if *pprofAddr != "" {
		go func() {
			log.Printf("[Server] pprof listening on %s", *pprofAddr)
			if err := http.ListenAndServe(*pprofAddr, nil); err != nil {
				log.Printf("[Server] pprof error: %v", err)
			}
		}()
	}

	// 1. 初始化 DB 连接
	manager, err := db.NewManager(cfg.MySQLDSN, cfg.RedisAddr, cfg.RedisPass)
	if err != nil {
		log.Fatalf("[Server] failed to init DB: %v", err)
	}
	defer manager.Close()

	// 2. 创建 Scene 管理器
	sceneMgr := scene.NewSceneManager(cfg.MapWidth, cfg.MapHeight, cfg.GridSize)

	// 3. 创建 Scene 服务
	sceneSrv := scene.NewServer(cfg.SceneListenAddr(), manager, sceneMgr)

	// 4. 创建 Lobby 服务（SceneAPI 暂时为 nil，后续注入）
	lobbySrv := lobby.NewServer(cfg.LobbyListenAddr(), manager)

	// 5. 互相注入接口引用（避免循环依赖）
	lobbySrv.SetSceneAPI(sceneSrv)
	sceneSrv.SetLobbyAPI(lobbySrv)

	// 6. 启动所有服务
	if err := sceneSrv.Start(); err != nil {
		log.Fatalf("[Server] failed to start Scene service: %v", err)
	}

	if err := lobbySrv.Start(); err != nil {
		log.Fatalf("[Server] failed to start Lobby service: %v", err)
	}

	log.Println("[Server] all services started!")
	log.Printf("[Server] Lobby: %s, Scene: %s", cfg.LobbyListenAddr(), cfg.SceneListenAddr())

	// 7. 等待退出信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log.Printf("[Server] received signal %v, shutting down...", sig)

	// 8. 优雅关闭
	lobbySrv.Stop()
	sceneSrv.Stop()
	log.Println("[Server] shutdown complete.")
}
