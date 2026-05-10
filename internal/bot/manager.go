package bot

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// Metrics 测试指标汇总
type Metrics struct {
	TotalBots       int
	LoginSuccess    int
	LoginFail       int
	EnterSuccess    int
	EnterFail       int
	LoginAvgMs      float64
	LoginP50Ms      float64
	LoginP95Ms      float64
	LoginP99Ms      float64
	EnterAvgMs      float64
	TotalMoveSent   int64
	TotalBroadcastRecv int64
	TotalAOIEnter   int64
	TotalAOILeave   int64
	Duration        time.Duration
}

// Manager 管理多个 Bot 进行压测
type Manager struct {
	serverAddr string
	bots       []*Client
}

func NewManager(serverAddr string) *Manager {
	return &Manager{
		serverAddr: serverAddr,
	}
}

// StartBots 启动指定数量的 Bot
func (m *Manager) StartBots(count int, ratePerSec int) {
	log.Printf("[BotManager] starting %d bots at %d/sec...", count, ratePerSec)

	m.bots = make([]*Client, count)
	var wg sync.WaitGroup

	interval := time.Second / time.Duration(ratePerSec)
	if interval < time.Millisecond {
		interval = time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for i := 0; i < count; i++ {
		m.bots[i] = NewClient(i+1, m.serverAddr)

		wg.Add(1)
		go func(bot *Client) {
			defer wg.Done()
			bot.Run()
		}(m.bots[i])

		<-ticker.C
	}
	wg.Wait()
	log.Printf("[BotManager] all %d bots started", count)
}

// StartMoving 让已在场景中的 Bot 开始随机移动
func (m *Manager) StartMoving(interval time.Duration, duration time.Duration) {
	log.Printf("[BotManager] starting movement for %d bots, interval=%v, duration=%v",
		len(m.bots), interval, duration)

	// 等待所有 bot 进入场景
	for i := 0; i < 50; i++ {
		allReady := true
		for _, b := range m.bots {
			if b.State() != StateInScene {
				allReady = false
				break
			}
		}
		if allReady {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	var wg sync.WaitGroup
	deadline := time.Now().Add(duration)

	inScene := 0
	for _, bot := range m.bots {
		if bot.State() != StateInScene {
			log.Printf("[BotManager] bot %d not in scene (state=%d), skipping", bot.ID, bot.State())
			continue
		}
		inScene++
		wg.Add(1)
		go func(b *Client) {
			defer wg.Done()
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					if time.Now().After(deadline) || !b.IsRunning() {
						return
					}
					b.SendMove()
				}
			}
		}(bot)
	}
	wg.Wait()
	log.Printf("[BotManager] movement finished (%d bots in scene)", inScene)
}

// AllBots 返回所有 bot 的切片
func (m *Manager) AllBots() []*Client {
	return m.bots
}

// LogoutAll 让所有 Bot 登出
func (m *Manager) LogoutAll() {
	log.Printf("[BotManager] logging out all bots...")
	var wg sync.WaitGroup
	for _, bot := range m.bots {
		wg.Add(1)
		go func(b *Client) {
			defer wg.Done()
			if b.State() == StateInScene || b.State() == StateLogin {
				b.Logout()
			} else {
				b.cleanup()
			}
		}(bot)
	}
	wg.Wait()
	log.Printf("[BotManager] all bots logged out")
}

// CollectMetrics 收集测试指标
func (m *Manager) CollectMetrics() *Metrics {
	met := &Metrics{TotalBots: len(m.bots)}

	var totalLogin, totalEnter time.Duration
	loginLatencies := make([]float64, 0)
	enterLatencies := make([]float64, 0)

	for _, bot := range m.bots {
		if bot.LoginErr == nil && bot.LoginLatency > 0 {
			met.LoginSuccess++
			totalLogin += bot.LoginLatency
			loginLatencies = append(loginLatencies, float64(bot.LoginLatency.Milliseconds()))
		} else {
			met.LoginFail++
		}

		if bot.EnterErr == nil && bot.EnterLatency > 0 {
			met.EnterSuccess++
			totalEnter += bot.EnterLatency
			enterLatencies = append(enterLatencies, float64(bot.EnterLatency.Milliseconds()))
		} else {
			met.EnterFail++
		}

		met.TotalMoveSent += bot.MoveSent.Load()
		met.TotalBroadcastRecv += bot.BroadcastRecv.Load()
		met.TotalAOIEnter += bot.AOIEnterRecv.Load()
		met.TotalAOILeave += bot.AOILeaveRecv.Load()
	}

	// 计算延迟百分位
	if len(loginLatencies) > 0 {
		sortFloats(loginLatencies)
		met.LoginAvgMs = float64(totalLogin.Milliseconds()) / float64(len(loginLatencies))
		met.LoginP50Ms = percentile(loginLatencies, 50)
		met.LoginP95Ms = percentile(loginLatencies, 95)
		met.LoginP99Ms = percentile(loginLatencies, 99)
	}

	return met
}

// PrintMetrics 打印测试报告
func (m *Manager) PrintMetrics(duration time.Duration) {
	met := m.CollectMetrics()
	met.Duration = duration

	fmt.Println("\n========= Test Report =========")
	fmt.Printf("Total Bots:        %d\n", met.TotalBots)
	fmt.Printf("Duration:          %v\n", met.Duration)
	fmt.Printf("Login Success:     %d/%d (%.1f%%)\n",
		met.LoginSuccess, met.TotalBots, pct(met.LoginSuccess, met.TotalBots))
	fmt.Printf("Enter Success:     %d/%d (%.1f%%)\n",
		met.EnterSuccess, met.TotalBots, pct(met.EnterSuccess, met.TotalBots))
	fmt.Printf("Login Avg:         %.1f ms\n", met.LoginAvgMs)
	fmt.Printf("Login P50/P95/P99: %.0f / %.0f / %.0f ms\n",
		met.LoginP50Ms, met.LoginP95Ms, met.LoginP99Ms)
	fmt.Printf("Move Sent:         %d\n", met.TotalMoveSent)
	fmt.Printf("Broadcast Recv:    %d\n", met.TotalBroadcastRecv)
	fmt.Printf("AOI Enter:         %d\n", met.TotalAOIEnter)
	fmt.Printf("AOI Leave:         %d\n", met.TotalAOILeave)
	fmt.Println("===============================")
}

// ========== 辅助函数 ==========

func pct(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b) * 100
}

func sortFloats(data []float64) {
	for i := 0; i < len(data); i++ {
		for j := i + 1; j < len(data); j++ {
			if data[j] < data[i] {
				data[i], data[j] = data[j], data[i]
			}
		}
	}
}

func percentile(data []float64, p int) float64 {
	if len(data) == 0 {
		return 0
	}
	idx := (p * len(data)) / 100
	if idx >= len(data) {
		idx = len(data) - 1
	}
	return data[idx]
}
