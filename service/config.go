package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultListenAddr         = ":8080"
	defaultPreferredCSV       = "preferred_ips.csv"
	defaultPreferredTopN      = 20
	defaultReloadInterval     = 5 * time.Second
	defaultSpeedtestInterval  = 30 * time.Minute
	defaultSubscriptionPath   = "/sub/passwall"
	defaultSubscriptionType   = "text/plain; charset=utf-8"
	defaultIPPlaceholder      = "{{IP}}"
	defaultIPListPlaceholder  = "{{IP_LIST}}"
	defaultIPListSeparator    = ","
	defaultSubscriptionPolicy = "first"
)

type Config struct {
	Listen               string               `json:"listen"`
	EnableSubscription   bool                 `json:"enable_subscription"`
	EnableSpeedtest      bool                 `json:"enable_speedtest"`
	ConfigReloadInterval string               `json:"config_reload_interval"`
	PreferredIPCSV       string               `json:"preferred_ip_csv"`
	PreferredTopN        int                  `json:"preferred_top_n"`
	SpeedtestInterval    string               `json:"speedtest_interval"`
	SpeedtestOnStart     bool                 `json:"speedtest_on_start"`
	Speedtest            SpeedtestConfig      `json:"speedtest"`
	Subscriptions        []SubscriptionConfig `json:"subscriptions"`

	reloadInterval time.Duration `json:"-"`
	speedtestEvery time.Duration `json:"-"`
	configDir      string        `json:"-"`
}

type SpeedtestConfig struct {
	Routines            int     `json:"routines"`
	PingTimes           int     `json:"ping_times"`
	TestCount           int     `json:"test_count"`
	DownloadTimeSeconds int     `json:"download_time_seconds"`
	TCPPort             int     `json:"tcp_port"`
	URL                 string  `json:"url"`
	Httping             bool    `json:"httping"`
	HttpingCode         int     `json:"httping_code"`
	HttpingCFColo       string  `json:"httping_cfcolo"`
	MaxDelayMS          int     `json:"max_delay_ms"`
	MinDelayMS          int     `json:"min_delay_ms"`
	MaxLossRate         float64 `json:"max_loss_rate"`
	MinSpeedMB          float64 `json:"min_speed_mb"`
	IPFile              string  `json:"ip_file"`
	IPText              string  `json:"ip_text"`
	Output              string  `json:"output"`
	PrintNum            int     `json:"print_num"`
	DisableDownload     bool    `json:"disable_download"`
	TestAllIP           bool    `json:"test_all_ip"`
	Debug               bool    `json:"debug"`
}

type SubscriptionConfig struct {
	Name               string            `json:"name"`
	Path               string            `json:"path"`
	Template           string            `json:"template"`
	TemplateFile       string            `json:"template_file"`
	TokenTemplateFiles map[string]string `json:"token_template_files"`
	ContentType        string            `json:"content_type"`
	Base64             bool              `json:"base64"`
	IPPlaceholder      string            `json:"ip_placeholder"`
	IPListPlaceholder  string            `json:"ip_list_placeholder"`
	IPListSeparator    string            `json:"ip_list_separator"`
	UseTopN            int               `json:"use_top_n"`
	IPStrategy         string            `json:"ip_strategy"`

	templateContent    string            `json:"-"`
	templatePath       string            `json:"-"`
	tokenTemplatePaths map[string]string `json:"-"`
}

func defaultSpeedtestConfig() SpeedtestConfig {
	return SpeedtestConfig{
		Routines:            200,
		PingTimes:           4,
		TestCount:           10,
		DownloadTimeSeconds: 10,
		TCPPort:             443,
		URL:                 "https://cf.xiu2.xyz/url",
		Httping:             false,
		HttpingCode:         0,
		HttpingCFColo:       "",
		MaxDelayMS:          9999,
		MinDelayMS:          0,
		MaxLossRate:         1,
		MinSpeedMB:          0,
		IPFile:              "ip.txt",
		IPText:              "",
		Output:              "result.csv",
		PrintNum:            0,
		DisableDownload:     false,
		TestAllIP:           false,
		Debug:               false,
	}
}

func defaultConfig() *Config {
	return &Config{
		Listen:               defaultListenAddr,
		EnableSubscription:   true,
		EnableSpeedtest:      true,
		ConfigReloadInterval: defaultReloadInterval.String(),
		PreferredIPCSV:       defaultPreferredCSV,
		PreferredTopN:        defaultPreferredTopN,
		SpeedtestInterval:    defaultSpeedtestInterval.String(),
		SpeedtestOnStart:     true,
		Speedtest:            defaultSpeedtestConfig(),
		Subscriptions: []SubscriptionConfig{
			{
				Name:              "passwall",
				Path:              defaultSubscriptionPath,
				Template:          defaultIPPlaceholder,
				ContentType:       defaultSubscriptionType,
				IPPlaceholder:     defaultIPPlaceholder,
				IPListPlaceholder: defaultIPListPlaceholder,
				IPListSeparator:   defaultIPListSeparator,
				UseTopN:           1,
				IPStrategy:        defaultSubscriptionPolicy,
			},
		},
	}
}

func loadConfigFromFile(path string) (*Config, time.Time, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, time.Time{}, err
	}

	cfg := defaultConfig()
	if err := json.Unmarshal(body, cfg); err != nil {
		return nil, time.Time{}, fmt.Errorf("解析配置失败: %w", err)
	}
	cfg.configDir = filepath.Dir(path)
	if err := cfg.normalizeAndValidate(); err != nil {
		return nil, time.Time{}, err
	}

	stat, err := os.Stat(path)
	if err != nil {
		return nil, time.Time{}, err
	}
	return cfg, stat.ModTime(), nil
}

func (c *Config) normalizeAndValidate() error {
	if c.Listen == "" {
		c.Listen = defaultListenAddr
	}
	if c.PreferredIPCSV == "" {
		c.PreferredIPCSV = defaultPreferredCSV
	}
	if !filepath.IsAbs(c.PreferredIPCSV) {
		c.PreferredIPCSV = filepath.Join(c.configDir, c.PreferredIPCSV)
	}
	if c.PreferredTopN <= 0 {
		c.PreferredTopN = defaultPreferredTopN
	}

	reloadInterval, err := parseDurationWithDefault(c.ConfigReloadInterval, defaultReloadInterval)
	if err != nil {
		return fmt.Errorf("config_reload_interval 配置错误: %w", err)
	}
	c.reloadInterval = reloadInterval

	speedtestEvery, err := parseDurationWithDefault(c.SpeedtestInterval, defaultSpeedtestInterval)
	if err != nil {
		return fmt.Errorf("speedtest_interval 配置错误: %w", err)
	}
	c.speedtestEvery = speedtestEvery

	if c.Speedtest.Routines <= 0 {
		c.Speedtest.Routines = 200
	}
	if c.Speedtest.PingTimes <= 0 {
		c.Speedtest.PingTimes = 4
	}
	if c.Speedtest.TestCount <= 0 {
		c.Speedtest.TestCount = 10
	}
	if c.Speedtest.DownloadTimeSeconds <= 0 {
		c.Speedtest.DownloadTimeSeconds = 10
	}
	if c.Speedtest.TCPPort <= 0 || c.Speedtest.TCPPort >= 65535 {
		c.Speedtest.TCPPort = 443
	}
	if c.Speedtest.URL == "" {
		c.Speedtest.URL = "https://cf.xiu2.xyz/url"
	}
	if c.Speedtest.MaxDelayMS <= 0 {
		c.Speedtest.MaxDelayMS = 9999
	}
	if c.Speedtest.MinDelayMS < 0 {
		c.Speedtest.MinDelayMS = 0
	}
	if c.Speedtest.MinDelayMS > c.Speedtest.MaxDelayMS {
		c.Speedtest.MinDelayMS, c.Speedtest.MaxDelayMS = c.Speedtest.MaxDelayMS, c.Speedtest.MinDelayMS
	}
	if c.Speedtest.MaxLossRate < 0 || c.Speedtest.MaxLossRate > 1 {
		c.Speedtest.MaxLossRate = 1
	}
	if c.Speedtest.MinSpeedMB < 0 {
		c.Speedtest.MinSpeedMB = 0
	}
	if c.Speedtest.IPFile == "" && c.Speedtest.IPText == "" {
		c.Speedtest.IPFile = "ip.txt"
	}
	if c.Speedtest.IPFile != "" && !filepath.IsAbs(c.Speedtest.IPFile) {
		c.Speedtest.IPFile = filepath.Join(c.configDir, c.Speedtest.IPFile)
	}
	if c.Speedtest.Output != "" && !filepath.IsAbs(c.Speedtest.Output) {
		c.Speedtest.Output = filepath.Join(c.configDir, c.Speedtest.Output)
	}

	for i := range c.Subscriptions {
		s := &c.Subscriptions[i]
		if s.Path == "" {
			if s.Name == "" {
				s.Path = defaultSubscriptionPath
			} else {
				s.Path = "/sub/" + strings.TrimPrefix(s.Name, "/")
			}
		}
		if !strings.HasPrefix(s.Path, "/") {
			return fmt.Errorf("subscriptions[%d].path 必须以 / 开头", i)
		}
		if s.ContentType == "" {
			s.ContentType = defaultSubscriptionType
		}
		if s.IPPlaceholder == "" {
			s.IPPlaceholder = defaultIPPlaceholder
		}
		if s.IPListPlaceholder == "" {
			s.IPListPlaceholder = defaultIPListPlaceholder
		}
		if s.IPListSeparator == "" {
			s.IPListSeparator = defaultIPListSeparator
		}
		if s.UseTopN <= 0 {
			s.UseTopN = c.PreferredTopN
		}
		if s.UseTopN <= 0 {
			s.UseTopN = 1
		}
		if s.IPStrategy == "" {
			s.IPStrategy = defaultSubscriptionPolicy
		}

		if len(s.TokenTemplateFiles) > 0 {
			s.tokenTemplatePaths = make(map[string]string, len(s.TokenTemplateFiles))
			for token, templateFile := range s.TokenTemplateFiles {
				token = strings.TrimSpace(token)
				if token == "" {
					return fmt.Errorf("subscriptions[%d].token_template_files 存在空 token", i)
				}
				templateFile = strings.TrimSpace(templateFile)
				if templateFile == "" {
					return fmt.Errorf("subscriptions[%d].token_template_files[%s] 模板路径为空", i, token)
				}
				tplPath := templateFile
				if !filepath.IsAbs(tplPath) {
					tplPath = filepath.Join(c.configDir, tplPath)
				}
				if _, err := os.ReadFile(tplPath); err != nil {
					return fmt.Errorf("读取 token 订阅模板失败 (%s): %w", tplPath, err)
				}
				s.tokenTemplatePaths[token] = tplPath
			}
		}

		// template_file 优先于 template。若已配置 token_template_files，则允许无默认模板（token-only 模式）。
		if s.TemplateFile != "" {
			tplPath := s.TemplateFile
			if !filepath.IsAbs(tplPath) {
				tplPath = filepath.Join(c.configDir, tplPath)
			}
			s.templatePath = tplPath
			body, err := os.ReadFile(tplPath)
			if err != nil {
				return fmt.Errorf("读取订阅模板失败 (%s): %w", tplPath, err)
			}
			s.templateContent = string(body)
		} else if s.Template != "" {
			s.templateContent = s.Template
		} else if len(s.tokenTemplatePaths) == 0 {
			return fmt.Errorf("subscriptions[%d] 缺少 template 或 template_file（或 token_template_files）", i)
		}
	}

	return nil
}

func parseDurationWithDefault(input string, fallback time.Duration) (time.Duration, error) {
	if strings.TrimSpace(input) == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(input)
	if err != nil {
		return 0, err
	}
	if d <= 0 {
		return 0, fmt.Errorf("时长必须大于 0")
	}
	return d, nil
}

func (c *Config) Clone() *Config {
	if c == nil {
		return nil
	}
	clone := *c
	if c.Subscriptions != nil {
		clone.Subscriptions = append([]SubscriptionConfig(nil), c.Subscriptions...)
	}
	return &clone
}

func (c *Config) ReloadEvery() time.Duration {
	if c == nil || c.reloadInterval <= 0 {
		return defaultReloadInterval
	}
	return c.reloadInterval
}

func (c *Config) SpeedtestEvery() time.Duration {
	if c == nil || c.speedtestEvery <= 0 {
		return defaultSpeedtestInterval
	}
	return c.speedtestEvery
}

func (c *Config) FindSubscription(path string) (SubscriptionConfig, bool) {
	for _, sub := range c.Subscriptions {
		if sub.Path == path {
			return sub, true
		}
	}
	return SubscriptionConfig{}, false
}

func (s SubscriptionConfig) TemplateContent() string {
	return s.templateContent
}

func (s SubscriptionConfig) LoadTemplateContent() (string, error) {
	if s.templatePath != "" {
		body, err := os.ReadFile(s.templatePath)
		if err != nil {
			return "", fmt.Errorf("读取订阅模板失败 (%s): %w", s.templatePath, err)
		}
		return string(body), nil
	}
	if s.templateContent != "" {
		return s.templateContent, nil
	}
	if s.Template != "" {
		return s.Template, nil
	}
	return "", fmt.Errorf("订阅模板为空")
}

func (s SubscriptionConfig) LoadTemplateContentForToken(token string) (string, error) {
	token = strings.TrimSpace(token)
	if len(s.tokenTemplatePaths) > 0 {
		if token == "" {
			return "", fmt.Errorf("x-token 无效")
		}
		if path, ok := s.tokenTemplatePaths[token]; ok {
			body, err := os.ReadFile(path)
			if err != nil {
				return "", fmt.Errorf("读取 token 订阅模板失败 (%s): %w", path, err)
			}
			return string(body), nil
		}
		return "", fmt.Errorf("x-token 无效")
	}
	return s.LoadTemplateContent()
}
