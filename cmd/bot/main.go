package main

import (
	"flag"
	"log"
	"net"
	"time"

	"gz14/internal/bot"
)

func main() {
	serverAddr := flag.String("addr", "127.0.0.1:19001", "server address")
	bots := flag.Int("bots", 10, "number of bots")
	rate := flag.Int("rate", 100, "bots start rate (per second)")
	moveInterval := flag.Int("move-ms", 200, "move interval in ms")
	duration := flag.Int("duration-sec", 30, "test duration in seconds")
	mode := flag.String("mode", "full", "test mode: full|login|move|relogin")
	flag.Parse()

	log.Printf("[Bot] starting test: addr=%s bots=%d rate=%d mode=%s",
		*serverAddr, *bots, *rate, *mode)

	manager := bot.NewManager(*serverAddr)

	switch *mode {
	case "login":
		// 仅测试登录
		start := time.Now()
		manager.StartBots(*bots, *rate)
		elapsed := time.Since(start)
		manager.PrintMetrics(elapsed)
		manager.LogoutAll()

	case "move":
		// 测试登录 + 移动
		start := time.Now()
		manager.StartBots(*bots, *rate)
		manager.StartMoving(time.Duration(*moveInterval)*time.Millisecond, time.Duration(*duration)*time.Second)
		elapsed := time.Since(start)
		manager.PrintMetrics(elapsed)
		manager.LogoutAll()

	case "relogin":
		// 反复登录登出
		rounds := 3
		for i := 0; i < rounds; i++ {
			log.Printf("[Bot] relogin round %d/%d", i+1, rounds)
			start := time.Now()
			manager.StartBots(*bots, *rate)
			runtime := time.Duration(*duration) * time.Second / time.Duration(rounds)
			manager.StartMoving(200*time.Millisecond, runtime)
			manager.LogoutAll()
			elapsed := time.Since(start)
			manager.PrintMetrics(elapsed)
		}

	case "reconnect":
		// 断线重连测试
		start := time.Now()
		manager.StartBots(*bots, *rate)
		log.Printf("[Bot] all bots in scene, testing reconnect...")

		// 模拟断线：关闭所有连接
		for _, b := range manager.AllBots() {
			if b.State() == bot.StateInScene {
				b.CloseConn()
				b.SetState(bot.StateInit)
			}
		}
		log.Printf("[Bot] connections closed, waiting 2s...")
		time.Sleep(2 * time.Second)

		// 重新连接并发送重连请求
		reconnOK := 0
		reconnFail := 0
		for _, b := range manager.AllBots() {
			if b.RoleID == 0 || b.Token == "" {
				continue
			}
			rawConn, err := net.DialTimeout("tcp", *serverAddr, 5*time.Second)
			if err != nil {
				reconnFail++
				continue
			}
			// 新连接替换旧连接
			b.ReplaceConn(rawConn)
			if err := b.DoReconnect(); err != nil {
				log.Printf("[Bot] bot %d reconnect failed: %v", b.ID, err)
				reconnFail++
			} else {
				reconnOK++
			}
		}
		elapsed := time.Since(start)
		log.Printf("[Bot] reconnect test: ok=%d fail=%d total=%d elapsed=%v",
			reconnOK, reconnFail, reconnOK+reconnFail, elapsed)

	case "full":
		fallthrough
	default:
		// 完整测试：登录 → 移动 → 登出
		start := time.Now()
		manager.StartBots(*bots, *rate)
		manager.StartMoving(time.Duration(*moveInterval)*time.Millisecond, time.Duration(*duration)*time.Second)
		elapsed := time.Since(start)
		manager.PrintMetrics(elapsed)
		manager.LogoutAll()
	}

	log.Println("[Bot] test finished")
}
