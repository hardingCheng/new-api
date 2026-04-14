package controller

import (
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func GetMonitorLogs(c *gin.Context) {
	tokenId := c.GetInt("token_id")
	if tokenId == 0 {
		common.ApiErrorMsg(c, "无法获取令牌信息")
		return
	}

	modelName := c.Query("model_name")
	if modelName == "" {
		common.ApiErrorMsg(c, "model_name 为必传参数")
		return
	}

	var startTimestamp, endTimestamp int64
	var bucketSeconds int64

	hoursStr := c.Query("hours")
	startStr := c.Query("start_timestamp")
	endStr := c.Query("end_timestamp")

	if hoursStr != "" {
		hours, err := strconv.Atoi(hoursStr)
		if err != nil || hours < 1 || hours > 72 {
			common.ApiErrorMsg(c, "hours 参数范围为 1~72")
			return
		}
		now := time.Now().Unix()
		endTimestamp = now
		startTimestamp = now - int64(hours)*3600
	} else if startStr != "" && endStr != "" {
		var err error
		startTimestamp, err = strconv.ParseInt(startStr, 10, 64)
		if err != nil {
			common.ApiErrorMsg(c, "start_timestamp 参数无效")
			return
		}
		endTimestamp, err = strconv.ParseInt(endStr, 10, 64)
		if err != nil {
			common.ApiErrorMsg(c, "end_timestamp 参数无效")
			return
		}
		if endTimestamp <= startTimestamp {
			common.ApiErrorMsg(c, "end_timestamp 必须大于 start_timestamp")
			return
		}
		maxRange := int64(72 * 3600)
		if endTimestamp-startTimestamp > maxRange {
			common.ApiErrorMsg(c, "时间范围不能超过 72 小时")
			return
		}
	} else {
		// Default: last 24 hours
		now := time.Now().Unix()
		endTimestamp = now
		startTimestamp = now - 24*3600
	}

	// Determine bucket size
	bucketStr := c.Query("bucket_seconds")
	if bucketStr != "" {
		var err error
		bucketSeconds, err = strconv.ParseInt(bucketStr, 10, 64)
		if err != nil || bucketSeconds < 60 || bucketSeconds > 86400 {
			common.ApiErrorMsg(c, "bucket_seconds 参数范围为 60~86400")
			return
		}
	} else {
		timeRange := endTimestamp - startTimestamp
		if timeRange <= 3600 {
			bucketSeconds = 300
		} else {
			bucketSeconds = 3600
		}
	}

	data, err := model.GetMonitorData(tokenId, modelName, startTimestamp, endTimestamp, bucketSeconds)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, data)
}
