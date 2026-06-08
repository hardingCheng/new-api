package controller

import (
	"fmt"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type channelBreakerStatusResponse struct {
	service.ChannelBreakerStatus
	ChannelName   string `json:"channel_name,omitempty"`
	ChannelGroup  string `json:"channel_group,omitempty"`
	ChannelStatus int    `json:"channel_status"`
	ChannelType   int    `json:"channel_type"`
	Priority      int64  `json:"priority"`
	Weight        uint   `json:"weight"`
	IsMultiKey    bool   `json:"is_multi_key"`
	KeyIndex      *int   `json:"key_index,omitempty"`
}

type clearChannelBreakerRequest struct {
	StateKey string `json:"state_key"`
}

func GetChannelBreakerStatuses(c *gin.Context) {
	statuses := service.ListChannelBreakerStatuses()
	channelIds := make([]int, 0, len(statuses))
	seen := make(map[int]bool)
	for _, status := range statuses {
		if status.ChannelId <= 0 || seen[status.ChannelId] {
			continue
		}
		seen[status.ChannelId] = true
		channelIds = append(channelIds, status.ChannelId)
	}

	channelsById := make(map[int]*model.Channel)
	if len(channelIds) > 0 {
		channels, err := model.GetChannelsByIds(channelIds)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		for _, channel := range channels {
			channelsById[channel.Id] = channel
		}
	}

	resp := make([]channelBreakerStatusResponse, 0, len(statuses))
	for _, status := range statuses {
		item := channelBreakerStatusResponse{ChannelBreakerStatus: status}
		if channel := channelsById[status.ChannelId]; channel != nil {
			item.ChannelName = channel.Name
			item.ChannelGroup = channel.Group
			item.ChannelStatus = channel.Status
			item.ChannelType = channel.Type
			item.IsMultiKey = channel.ChannelInfo.IsMultiKey
			if channel.Priority != nil {
				item.Priority = *channel.Priority
			}
			if channel.Weight != nil {
				item.Weight = *channel.Weight
			}
			if status.KeyHash != "" {
				if keyIndex, ok := findChannelBreakerKeyIndex(channel, status.KeyHash); ok {
					item.KeyIndex = &keyIndex
				}
			}
		}
		resp = append(resp, item)
	}

	common.ApiSuccess(c, gin.H{"items": resp})
}

func GetChannelBreakerLogs(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	logs, total, err := model.GetChannelBreakerLogs(pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(logs)
	common.ApiSuccess(c, pageInfo)
}

func ClearChannelBreakerStatus(c *gin.Context) {
	var req clearChannelBreakerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if req.StateKey == "" {
		common.ApiErrorMsg(c, "state_key 不能为空")
		return
	}
	if !service.ClearChannelBreakerByStateKey(req.StateKey) {
		common.ApiErrorMsg(c, "熔断状态不存在或已恢复")
		return
	}
	common.SysLog(fmt.Sprintf("管理员手动解除熔断：%s", req.StateKey))
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "已解除熔断"})
}

func findChannelBreakerKeyIndex(channel *model.Channel, keyHash string) (int, bool) {
	for index, key := range channel.GetKeys() {
		if service.ChannelBreakerKeyHash(key) == keyHash {
			return index, true
		}
	}
	return 0, false
}
