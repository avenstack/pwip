package service

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/avenstack/pwip/utils"
)

var preferredCSVHeader = []string{"IP 地址", "平均延迟(ms)", "下载速度(MB/s)", "丢包率", "地区码", "更新时间", "更新次数"}

type PreferredIPRecord struct {
	IP              string
	DelayMS         float64
	DownloadSpeedMB float64
	LossRate        float64
	Colo            string
	UpdatedAt       time.Time
	Count           int
}

func LoadPreferredIPCSV(path string) ([]PreferredIPRecord, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []PreferredIPRecord{}, nil
		}
		return nil, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	records := make([]PreferredIPRecord, 0, len(rows))
	for i, row := range rows {
		if len(row) < 7 {
			continue
		}
		if i == 0 {
			header := strings.TrimSpace(row[0])
			if header == "IP 地址" || strings.EqualFold(header, "ip") || strings.EqualFold(header, "ip address") {
				continue
			}
		}

		delay, err := strconv.ParseFloat(strings.TrimSpace(row[1]), 64)
		if err != nil {
			continue
		}
		speed, err := strconv.ParseFloat(strings.TrimSpace(row[2]), 64)
		if err != nil {
			continue
		}
		loss, err := strconv.ParseFloat(strings.TrimSpace(row[3]), 64)
		if err != nil {
			continue
		}
		count, err := strconv.Atoi(strings.TrimSpace(row[6]))
		if err != nil {
			continue
		}
		updatedAt := parseTimestamp(strings.TrimSpace(row[5]))
		if count <= 0 {
			count = 1
		}

		records = append(records, PreferredIPRecord{
			IP:              strings.TrimSpace(row[0]),
			DelayMS:         delay,
			DownloadSpeedMB: speed,
			LossRate:        loss,
			Colo:            strings.TrimSpace(row[4]),
			UpdatedAt:       updatedAt,
			Count:           count,
		})
	}

	sortPreferredRecords(records)
	return records, nil
}

func UpdatePreferredIPCSV(path string, speedSet utils.DownloadSpeedSet, topN int, batchTime time.Time) ([]PreferredIPRecord, error) {
	if topN <= 0 {
		topN = defaultPreferredTopN
	}
	if batchTime.IsZero() {
		batchTime = time.Now()
	}

	existing, err := LoadPreferredIPCSV(path)
	if err != nil {
		return nil, err
	}
	recordMap := make(map[string]PreferredIPRecord, len(existing))
	for _, record := range existing {
		recordMap[record.IP] = record
	}

	candidates := toPreferredCandidates(speedSet)
	sortPreferredRecords(candidates)
	if len(candidates) > topN {
		candidates = candidates[:topN]
	}

	for _, candidate := range candidates {
		record, exists := recordMap[candidate.IP]
		if !exists {
			record = candidate
			record.Count = 1
		} else {
			record.DelayMS = candidate.DelayMS
			record.DownloadSpeedMB = candidate.DownloadSpeedMB
			record.LossRate = candidate.LossRate
			record.Colo = candidate.Colo
			record.Count++
		}
		record.UpdatedAt = batchTime
		recordMap[candidate.IP] = record
	}

	merged := make([]PreferredIPRecord, 0, len(recordMap))
	for _, record := range recordMap {
		merged = append(merged, record)
	}
	sortPreferredRecords(merged)
	if len(merged) > topN {
		merged = merged[:topN]
	}

	if err := writePreferredCSV(path, merged); err != nil {
		return nil, err
	}
	return merged, nil
}

func toPreferredCandidates(speedSet utils.DownloadSpeedSet) []PreferredIPRecord {
	records := make([]PreferredIPRecord, 0, len(speedSet))
	for _, item := range speedSet {
		if item.IP == nil {
			continue
		}
		lossRate := 0.0
		if item.Sended > 0 {
			lossRate = float64(item.Sended-item.Received) / float64(item.Sended)
		}
		colo := item.Colo
		if colo == "" {
			colo = "N/A"
		}
		records = append(records, PreferredIPRecord{
			IP:              item.IP.String(),
			DelayMS:         float64(item.Delay) / float64(time.Millisecond),
			DownloadSpeedMB: item.DownloadSpeed / 1024 / 1024,
			LossRate:        lossRate,
			Colo:            colo,
		})
	}
	return records
}

func sortPreferredRecords(records []PreferredIPRecord) {
	sort.Slice(records, func(i, j int) bool {
		if records[i].DownloadSpeedMB != records[j].DownloadSpeedMB {
			return records[i].DownloadSpeedMB > records[j].DownloadSpeedMB
		}
		if records[i].DelayMS != records[j].DelayMS {
			return records[i].DelayMS < records[j].DelayMS
		}
		if records[i].LossRate != records[j].LossRate {
			return records[i].LossRate < records[j].LossRate
		}
		if records[i].Count != records[j].Count {
			return records[i].Count > records[j].Count
		}
		return records[i].IP < records[j].IP
	})
}

func writePreferredCSV(path string, records []PreferredIPRecord) error {
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	tmpPath := path + ".tmp"
	file, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	writer := csv.NewWriter(file)
	if err := writer.Write(preferredCSVHeader); err != nil {
		_ = file.Close()
		return err
	}

	for _, record := range records {
		line := []string{
			record.IP,
			fmt.Sprintf("%.2f", record.DelayMS),
			fmt.Sprintf("%.2f", record.DownloadSpeedMB),
			fmt.Sprintf("%.2f", record.LossRate),
			record.Colo,
			record.UpdatedAt.Format(time.RFC3339),
			strconv.Itoa(record.Count),
		}
		if err := writer.Write(line); err != nil {
			_ = file.Close()
			return err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		_ = file.Close()
		return err
	}

	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	return nil
}

func parseTimestamp(input string) time.Time {
	if input == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, input); err == nil {
		return t
	}
	if t, err := time.Parse("2006-01-02 15:04:05", input); err == nil {
		return t
	}
	return time.Time{}
}
