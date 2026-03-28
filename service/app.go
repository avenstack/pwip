package service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/avenstack/pwip/task"
)

type ConfigManager struct {
	path    string
	logger  *log.Logger
	mu      sync.RWMutex
	config  *Config
	modTime time.Time
}

func NewConfigManager(path string, logger *log.Logger) (*ConfigManager, error) {
	cfg, modTime, err := loadConfigFromFile(path)
	if err != nil {
		return nil, err
	}
	return &ConfigManager{
		path:    path,
		logger:  logger,
		config:  cfg,
		modTime: modTime,
	}, nil
}

func (m *ConfigManager) Current() *Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config.Clone()
}

func (m *ConfigManager) ReloadIfChanged() (bool, error) {
	stat, err := os.Stat(m.path)
	if err != nil {
		return false, err
	}

	m.mu.RLock()
	lastMod := m.modTime
	m.mu.RUnlock()
	if !stat.ModTime().After(lastMod) {
		return false, nil
	}

	cfg, modTime, err := loadConfigFromFile(m.path)
	if err != nil {
		return false, err
	}

	m.mu.Lock()
	oldListen := m.config.Listen
	m.config = cfg
	m.modTime = modTime
	m.mu.Unlock()

	m.logger.Printf("配置热更新成功: %s", m.path)
	if oldListen != cfg.Listen {
		m.logger.Printf("listen 从 %s 变更为 %s，需重启服务后生效", oldListen, cfg.Listen)
	}
	return true, nil
}

type App struct {
	logger   *log.Logger
	manager  *ConfigManager
	random   *rand.Rand
	randMu   sync.Mutex
	rrMu     sync.Mutex
	rrCursor map[string]int
}

func Run(configPath string) error {
	logger := log.New(os.Stdout, "[passwall] ", log.LstdFlags)
	manager, err := NewConfigManager(configPath, logger)
	if err != nil {
		return err
	}

	app := &App{
		logger:   logger,
		manager:  manager,
		random:   rand.New(rand.NewSource(time.Now().UnixNano())),
		rrCursor: make(map[string]int),
	}
	return app.Run()
}

func (a *App) Run() error {
	task.InitRandSeed()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := a.manager.Current()
	server := &http.Server{
		Addr:         cfg.Listen,
		Handler:      http.HandlerFunc(a.handleHTTP),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		a.logger.Printf("订阅服务监听: %s", cfg.Listen)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	go a.configReloadLoop(ctx)
	go a.speedtestLoop(ctx)

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		a.logger.Println("服务已停止")
		return nil
	case err := <-errCh:
		stop()
		return err
	}
}

func (a *App) handleHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/healthz" {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
		return
	}

	cfg := a.manager.Current()
	if !cfg.EnableSubscription {
		http.Error(w, "subscription service disabled", http.StatusServiceUnavailable)
		return
	}

	rule, found := cfg.FindSubscription(r.URL.Path)
	if !found {
		http.NotFound(w, r)
		return
	}
	token := strings.TrimSpace(r.URL.Query().Get("x-token"))
	token = strings.TrimSuffix(token, ",")

	records, err := LoadPreferredIPCSV(cfg.PreferredIPCSV)
	if err != nil {
		a.logger.Printf("读取优选IP失败: %v", err)
		http.Error(w, "load preferred ip failed", http.StatusInternalServerError)
		return
	}

	content, err := a.renderSubscription(rule, records, cfg.PreferredTopN, token)
	if err != nil {
		a.logger.Printf("生成订阅失败(path=%s): %v", rule.Path, err)
		if err.Error() == "x-token 无效" {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", rule.ContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(content))
}

func (a *App) renderSubscription(rule SubscriptionConfig, records []PreferredIPRecord, defaultTopN int, token string) (string, error) {
	content, err := rule.LoadTemplateContentForToken(token)
	if err != nil {
		return "", err
	}

	if rule.IPPlaceholder == "" {
		rule.IPPlaceholder = defaultIPPlaceholder
	}
	if rule.IPListPlaceholder == "" {
		rule.IPListPlaceholder = defaultIPListPlaceholder
	}
	if rule.IPListSeparator == "" {
		rule.IPListSeparator = defaultIPListSeparator
	}
	if rule.UseTopN <= 0 {
		rule.UseTopN = defaultTopN
	}
	if rule.UseTopN <= 0 {
		rule.UseTopN = 1
	}

	selectedIP := ""
	if strings.Contains(content, rule.IPPlaceholder) {
		selectedIP = a.pickIP(rule, records)
		if selectedIP == "" {
			return "", fmt.Errorf("暂无优选IP数据")
		}
		content = strings.ReplaceAll(content, rule.IPPlaceholder, selectedIP)
	}

	if strings.Contains(content, rule.IPListPlaceholder) {
		limit := rule.UseTopN
		if len(records) < limit {
			limit = len(records)
		}
		if limit <= 0 {
			return "", fmt.Errorf("暂无优选IP数据")
		}

		ipList := make([]string, 0, limit)
		for i := 0; i < limit; i++ {
			ipList = append(ipList, records[i].IP)
		}
		content = strings.ReplaceAll(content, rule.IPListPlaceholder, strings.Join(ipList, rule.IPListSeparator))
	}

	if rule.Base64 {
		content = base64.StdEncoding.EncodeToString([]byte(content))
	}
	return content, nil
}

func (a *App) pickIP(rule SubscriptionConfig, records []PreferredIPRecord) string {
	if len(records) == 0 {
		return ""
	}
	strategy := strings.ToLower(strings.TrimSpace(rule.IPStrategy))
	switch strategy {
	case "random":
		a.randMu.Lock()
		idx := a.random.Intn(len(records))
		a.randMu.Unlock()
		return records[idx].IP
	case "round_robin":
		a.rrMu.Lock()
		idx := a.rrCursor[rule.Path] % len(records)
		a.rrCursor[rule.Path]++
		a.rrMu.Unlock()
		return records[idx].IP
	default:
		return records[0].IP
	}
}

func (a *App) configReloadLoop(ctx context.Context) {
	for {
		cfg := a.manager.Current()
		if !sleepOrDone(ctx, cfg.ReloadEvery()) {
			return
		}
		if _, err := a.manager.ReloadIfChanged(); err != nil {
			a.logger.Printf("配置热更新失败: %v", err)
		}
	}
}

func (a *App) speedtestLoop(ctx context.Context) {
	firstRound := true
	for {
		cfg := a.manager.Current()
		if !cfg.EnableSpeedtest {
			if !sleepOrDone(ctx, 10*time.Second) {
				return
			}
			continue
		}

		if firstRound {
			firstRound = false
			if cfg.SpeedtestOnStart {
				a.runSpeedtestJob(cfg)
			}
		}

		if !sleepOrDone(ctx, cfg.SpeedtestEvery()) {
			return
		}
		cfg = a.manager.Current()
		if cfg.EnableSpeedtest {
			a.runSpeedtestJob(cfg)
		}
	}
}

func (a *App) runSpeedtestJob(cfg *Config) {
	defer func() {
		if recoverErr := recover(); recoverErr != nil {
			a.logger.Printf("测速任务异常恢复: %v", recoverErr)
		}
	}()

	if err := RunSpeedtestAndUpdatePreferred(cfg, a.logger); err != nil {
		a.logger.Printf("测速任务失败: %v", err)
	}
}

func sleepOrDone(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
