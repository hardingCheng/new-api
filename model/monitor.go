package model

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
)

type MonitorSummary struct {
	ModelName          string  `json:"modelName"`
	TotalRecords       int64   `json:"totalRecords"`
	SuccessRecords     int64   `json:"successRecords"`
	FailedRecords      int64   `json:"failedRecords"`
	SuccessRate        float64 `json:"successRate"`
	AvgTime            float64 `json:"avgTime"`
	AvgTotalTokens     float64 `json:"avgTotalTokens"`
	TotalTokens        int64   `json:"totalTokens"`
	ActiveHours        int     `json:"activeHours"`
	PeakCount          int64   `json:"peakCount"`
	PeakHour           int64   `json:"peakHour"`
	UniqueUsers        int64   `json:"uniqueUsers"`
	AvgQuotaPerRequest float64 `json:"avgQuotaPerRequest"`
	TotalQuota         float64 `json:"totalQuota"`
	FirstUsedAt        int64   `json:"firstUsedAt"`
	LastUsedAt         int64   `json:"lastUsedAt"`
}

type HourlyStat struct {
	Hour    int64   `json:"hour"`
	Total   int64   `json:"total"`
	Success int64   `json:"success"`
	Failed  int64   `json:"failed"`
	AvgTime float64 `json:"avgTime"`
}

type MonitorData struct {
	Summary      *MonitorSummary `json:"summary"`
	HourlyStats  []HourlyStat   `json:"hourly_stats"`
	FailedLast5m int64           `json:"failed_last_5m"`
}

type summaryRow struct {
	TotalRecords   int64
	SuccessRecords int64
	FailedRecords  int64
	AvgTime        float64
	AvgTotalTokens float64
	TotalTokens    int64
	UniqueUsers    int64
	AvgQuota       float64
	TotalQuota     int64
	FirstUsedAt    int64
	LastUsedAt     int64
}

type hourlyRow struct {
	Hour    int64
	Total   int64
	Success int64
	Failed  int64
	AvgTime float64
}

func GetMonitorData(tokenId int, modelName string, startTimestamp, endTimestamp int64, bucketSeconds int64) (*MonitorData, error) {
	summary, err := getMonitorSummary(tokenId, modelName, startTimestamp, endTimestamp)
	if err != nil {
		return nil, err
	}

	hourlyStats, err := getHourlyStats(tokenId, modelName, startTimestamp, endTimestamp, bucketSeconds)
	if err != nil {
		return nil, err
	}

	failedLast5m, err := getFailedLast5m(tokenId, modelName)
	if err != nil {
		return nil, err
	}

	// Compute peak from hourly stats
	var peakCount int64
	var peakHour int64
	activeHours := 0
	for _, h := range hourlyStats {
		if h.Total > 0 {
			activeHours++
		}
		if h.Total > peakCount {
			peakCount = h.Total
			peakHour = h.Hour
		}
	}
	summary.ActiveHours = activeHours
	summary.PeakCount = peakCount
	summary.PeakHour = peakHour

	return &MonitorData{
		Summary:      summary,
		HourlyStats:  hourlyStats,
		FailedLast5m: failedLast5m,
	}, nil
}

func getMonitorSummary(tokenId int, modelName string, startTimestamp, endTimestamp int64) (*MonitorSummary, error) {
	var row summaryRow
	err := LOG_DB.Table("logs").
		Select(`COUNT(*) as total_records,
			SUM(CASE WHEN type = 2 THEN 1 ELSE 0 END) as success_records,
			SUM(CASE WHEN type = 5 THEN 1 ELSE 0 END) as failed_records,
			AVG(use_time) as avg_time,
			AVG(prompt_tokens + completion_tokens) as avg_total_tokens,
			SUM(prompt_tokens + completion_tokens) as total_tokens,
			COUNT(DISTINCT user_id) as unique_users,
			AVG(quota) as avg_quota,
			SUM(quota) as total_quota,
			MIN(created_at) as first_used_at,
			MAX(created_at) as last_used_at`).
		Where("token_id = ? AND model_name = ? AND created_at BETWEEN ? AND ? AND type IN (2, 5)",
			tokenId, modelName, startTimestamp, endTimestamp).
		Scan(&row).Error
	if err != nil {
		return nil, err
	}

	var successRate float64
	if row.TotalRecords > 0 {
		successRate = float64(row.SuccessRecords) / float64(row.TotalRecords) * 100
	}

	return &MonitorSummary{
		ModelName:          modelName,
		TotalRecords:       row.TotalRecords,
		SuccessRecords:     row.SuccessRecords,
		FailedRecords:      row.FailedRecords,
		SuccessRate:        successRate,
		AvgTime:            row.AvgTime / 1000,
		AvgTotalTokens:     row.AvgTotalTokens,
		TotalTokens:        row.TotalTokens,
		UniqueUsers:        row.UniqueUsers,
		AvgQuotaPerRequest: row.AvgQuota / common.QuotaPerUnit,
		TotalQuota:         float64(row.TotalQuota) / common.QuotaPerUnit,
		FirstUsedAt:        row.FirstUsedAt,
		LastUsedAt:         row.LastUsedAt,
	}, nil
}

func getHourlyStats(tokenId int, modelName string, startTimestamp, endTimestamp int64, bucketSeconds int64) ([]HourlyStat, error) {
	// Use fmt.Sprintf for GROUP BY since GORM's Group() doesn't support parameterized args.
	// bucketSeconds is a safe int64 value, no injection risk.
	bucketExpr := fmt.Sprintf("(created_at / %d) * %d", bucketSeconds, bucketSeconds)

	var rows []hourlyRow
	err := LOG_DB.Table("logs").
		Select(bucketExpr + ` as hour,
			COUNT(*) as total,
			SUM(CASE WHEN type = 2 THEN 1 ELSE 0 END) as success,
			SUM(CASE WHEN type = 5 THEN 1 ELSE 0 END) as failed,
			AVG(use_time) as avg_time`).
		Where("token_id = ? AND model_name = ? AND created_at BETWEEN ? AND ? AND type IN (2, 5)",
			tokenId, modelName, startTimestamp, endTimestamp).
		Group(bucketExpr).
		Order("hour ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	if rows == nil {
		rows = []hourlyRow{}
	}

	stats := make([]HourlyStat, len(rows))
	for i, r := range rows {
		stats[i] = HourlyStat{
			Hour:    r.Hour,
			Total:   r.Total,
			Success: r.Success,
			Failed:  r.Failed,
			AvgTime: r.AvgTime / 1000,
		}
	}
	return stats, nil
}

func getFailedLast5m(tokenId int, modelName string) (int64, error) {
	now := common.GetTimestamp()
	fiveMinAgo := now - 300

	var count int64
	err := LOG_DB.Table("logs").
		Where("token_id = ? AND model_name = ? AND type = 5 AND created_at >= ?",
			tokenId, modelName, fiveMinAgo).
		Count(&count).Error
	return count, err
}
