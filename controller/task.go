package controller

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// UpdateTaskBulk 薄入口，实际轮询逻辑在 service 层
func UpdateTaskBulk() {
	service.TaskPollingLoop()
}

func GetAllTask(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)

	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	// 解析其他查询参数
	queryParams := model.SyncTaskQueryParams{
		Platform:       constant.TaskPlatform(c.Query("platform")),
		TaskID:         c.Query("task_id"),
		Status:         c.Query("status"),
		Action:         c.Query("action"),
		ModelName:      strings.TrimSpace(c.Query("model_name")),
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
		ChannelID:      c.Query("channel_id"),
	}
	if username := strings.TrimSpace(c.Query("username")); username != "" {
		userIDs, err := model.SearchUserIDsByUsername(username, 1000)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		queryParams.UserIDs = userIDs
		if len(userIDs) == 0 {
			pageInfo.SetTotal(0)
			pageInfo.SetItems([]*dto.TaskDto{})
			common.ApiSuccess(c, pageInfo)
			return
		}
	}

	items := model.TaskGetAllTasks(pageInfo.GetStartIdx(), pageInfo.GetPageSize(), queryParams)
	total := model.TaskCountAllTasks(queryParams)
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(tasksToDto(items, true))
	common.ApiSuccess(c, pageInfo)
}

func GetUserTask(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)

	userId := c.GetInt("id")

	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)

	queryParams := model.SyncTaskQueryParams{
		Platform:       constant.TaskPlatform(c.Query("platform")),
		TaskID:         c.Query("task_id"),
		Action:         c.Query("action"),
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
	}

	items := model.TaskGetAllUserTask(userId, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), queryParams)
	total := model.TaskCountAllUserTask(userId, queryParams)
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(tasksToDto(items, false))
	common.ApiSuccess(c, pageInfo)
}

func GetModelQuotaPoolUsage(c *gin.Context) {
	userID := c.GetInt("id")
	includeAllUserPools := c.GetInt("role") >= common.RoleAdminUser
	common.ApiSuccess(c, service.GetVisibleModelQuotaPoolUsage(userID, includeAllUserPools))
}

func tasksToDto(tasks []*model.Task, fillUser bool) []*dto.TaskDto {
	var userIdMap map[int]*model.UserBase
	channelIdMap := make(map[int]string)
	if fillUser {
		userIdMap = make(map[int]*model.UserBase)
		userIds := types.NewSet[int]()
		for _, task := range tasks {
			userIds.Add(task.UserId)
		}
		for _, userId := range userIds.Items() {
			cacheUser, err := model.GetUserCache(userId)
			if err == nil {
				userIdMap[userId] = cacheUser
			}
		}
	}
	channelIds := types.NewSet[int]()
	for _, task := range tasks {
		if task.ChannelId != 0 {
			channelIds.Add(task.ChannelId)
		}
	}
	for _, channelId := range channelIds.Items() {
		channel, err := model.CacheGetChannel(channelId)
		if err == nil && channel != nil {
			channelIdMap[channelId] = channel.Name
		}
	}
	result := make([]*dto.TaskDto, len(tasks))
	for i, task := range tasks {
		if fillUser {
			if user, ok := userIdMap[task.UserId]; ok {
				task.Username = user.Username
			}
		}
		if channelName, ok := channelIdMap[task.ChannelId]; ok {
			task.ChannelName = channelName
		}
		dtoItem := relay.TaskModel2Dto(task)
		// 普通用户路径（非管理员）脱敏：移除计费/渠道/上游模型名等内部字段
		if !fillUser {
			redactTaskDtoForUser(dtoItem)
		}
		result[i] = dtoItem
	}
	return result
}

// redactTaskDtoForUser 移除普通用户不应看到的内部字段：
// 计费额度、归属渠道/分组、内部主键，以及上游真实模型名和上游内部 task_id。
func redactTaskDtoForUser(d *dto.TaskDto) {
	d.Quota = 0
	d.RefundQuota = 0
	// 注意：d.Key 是任务的数据库自增 ID 字符串（非 API 密钥），
	// 前端任务表格用它作为 rowKey，置空会导致行 key 冲突，故保留。
	d.Group = ""
	d.ChannelId = 0
	d.ChannelName = ""
	if props, ok := d.Properties.(model.Properties); ok {
		props.UpstreamModelName = ""
		d.Properties = props
	}
	d.Data = redactTaskDataForUser(d.Data, d.ModelName, d.TaskID)
}

// redactTaskDataForUser 脱敏原始上游响应体中的 model（替换为对外模型名）和
// task_id（替换为对外公开 ID）。
func redactTaskDataForUser(data json.RawMessage, originModel, publicTaskID string) json.RawMessage {
	if len(data) == 0 {
		return data
	}
	var m map[string]any
	if err := common.Unmarshal(data, &m); err != nil {
		return data
	}
	changed := false
	if _, ok := m["model"]; ok {
		if originModel != "" {
			m["model"] = originModel
		} else {
			delete(m, "model")
		}
		changed = true
	}
	if _, ok := m["task_id"]; ok {
		m["task_id"] = publicTaskID
		changed = true
	}
	if !changed {
		return data
	}
	b, err := common.Marshal(m)
	if err != nil {
		return data
	}
	return json.RawMessage(b)
}
