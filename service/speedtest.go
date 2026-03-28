package service

import (
	"log"
	"time"

	"github.com/avenstack/pwip/task"
	"github.com/avenstack/pwip/utils"
)

func RunSpeedtestAndUpdatePreferred(cfg *Config, logger *log.Logger) error {
	applySpeedtestConfig(cfg.Speedtest)

	batchTime := time.Now()
	logger.Printf("测速任务开始: timeout=%ds, test_count=%d", cfg.Speedtest.DownloadTimeSeconds, cfg.Speedtest.TestCount)
	pingData := task.NewPing().Run().FilterDelay().FilterLossRate()
	speedData := task.TestDownloadSpeed(pingData)
	utils.ExportCsv(speedData)
	if cfg.Speedtest.PrintNum > 0 {
		speedData.Print()
	}
	if len(speedData) == 0 {
		logger.Printf("测速任务完成，但没有可用 IP")
		return nil
	}

	records, err := UpdatePreferredIPCSV(cfg.PreferredIPCSV, speedData, cfg.PreferredTopN, batchTime)
	if err != nil {
		return err
	}
	logger.Printf("测速任务完成: 结果 %d 条，优选IP保留 %d 条，输出文件 %s", len(speedData), len(records), cfg.PreferredIPCSV)
	return nil
}

func applySpeedtestConfig(cfg SpeedtestConfig) {
	task.Routines = cfg.Routines
	task.PingTimes = cfg.PingTimes
	task.TestCount = cfg.TestCount
	task.TCPPort = cfg.TCPPort
	task.URL = cfg.URL
	task.Httping = cfg.Httping
	task.HttpingStatusCode = cfg.HttpingCode
	task.HttpingCFColo = cfg.HttpingCFColo
	task.HttpingCFColomap = task.MapColoMap()
	task.Disable = cfg.DisableDownload
	task.TestAll = cfg.TestAllIP
	task.MinSpeed = cfg.MinSpeedMB
	task.Timeout = time.Duration(cfg.DownloadTimeSeconds) * time.Second
	task.IPFile = cfg.IPFile
	task.IPText = cfg.IPText

	utils.InputMaxDelay = time.Duration(cfg.MaxDelayMS) * time.Millisecond
	utils.InputMinDelay = time.Duration(cfg.MinDelayMS) * time.Millisecond
	utils.InputMaxLossRate = float32(cfg.MaxLossRate)
	utils.Output = cfg.Output
	utils.PrintNum = cfg.PrintNum
	utils.Debug = cfg.Debug
}
