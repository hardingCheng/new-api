package controller

import (
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
	pageInfo.SetItems(tasksToDto(items, taskDtoOptions{
		adminView:             true,
		includeUpstreamTaskID: true,
	}))
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
	pageInfo.SetItems(tasksToDto(items, taskDtoOptions{}))
	common.ApiSuccess(c, pageInfo)
}

const (
	taskExportMaxRangeSeconds = int64(31 * 24 * 60 * 60)
	taskExportMaxRows         = 5000
)

// GetAllTaskExport 返回指定时间范围内的任务，供后台导出报表使用。
func GetAllTaskExport(c *gin.Context) {
	startTimestamp, err := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	if err != nil || startTimestamp <= 0 {
		common.ApiErrorMsg(c, "invalid start_timestamp")
		return
	}
	endTimestamp, err := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	if err != nil || endTimestamp <= 0 {
		common.ApiErrorMsg(c, "invalid end_timestamp")
		return
	}
	if endTimestamp < startTimestamp {
		common.ApiErrorMsg(c, "invalid time range")
		return
	}
	if endTimestamp-startTimestamp > taskExportMaxRangeSeconds {
		common.ApiErrorMsg(c, "task export time range cannot exceed 31 days")
		return
	}
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
			common.ApiSuccess(c, gin.H{"items": []*dto.TaskDto{}})
			return
		}
	}
	items, err := model.TaskGetAllTasksForExport(taskExportMaxRows+1, queryParams)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if len(items) > taskExportMaxRows {
		common.ApiErrorMsg(c, "task export exceeds 5000 rows; narrow the filters")
		return
	}
	common.ApiSuccess(c, gin.H{"items": tasksToDto(items, taskDtoOptions{
		adminView: true,
	})})
}

func GetModelQuotaPoolUsage(c *gin.Context) {
	userID := c.GetInt("id")
	includeAllUserPools := c.GetInt("role") >= common.RoleAdminUser && strings.TrimSpace(c.Query("scope")) != "self"
	common.ApiSuccess(c, service.GetVisibleModelQuotaPoolUsage(userID, includeAllUserPools))
}

type taskDtoOptions struct {
	adminView             bool
	includeUpstreamTaskID bool
}

func tasksToDto(tasks []*model.Task, options taskDtoOptions) []*dto.TaskDto {
	var userIdMap map[int]*model.UserBase
	channelIdMap := make(map[int]string)
	if options.adminView {
		userIdMap = make(map[int]*model.UserBase)
		userIds := types.NewSet[int]()
		for _, task := range tasks {
			if task.UserId > 0 {
				userIds.Add(task.UserId)
			}
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
		if options.adminView {
			if user, ok := userIdMap[task.UserId]; ok {
				task.Username = user.Username
			}
		}
		if channelName, ok := channelIdMap[task.ChannelId]; ok {
			task.ChannelName = channelName
		}
		dtoItem := relay.TaskModel2Dto(task)
		if options.includeUpstreamTaskID {
			dtoItem.UpstreamTaskID = task.GetUpstreamTaskID()
		}
		// 普通用户路径（非管理员）脱敏：移除计费/渠道/上游模型名等内部字段
		if !options.adminView {
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
	d.UpstreamTaskID = ""
	// 注意：d.Key 是任务的数据库自增 ID 字符串（非 API 密钥），
	// 前端任务表格用它作为 rowKey，置空会导致行 key 冲突，故保留。
	d.Group = ""
	d.ChannelId = 0
	d.ChannelName = ""
	publicModel := ""
	if props, ok := d.Properties.(model.Properties); ok {
		publicModel = strings.TrimSpace(props.OriginModelName)
		props.UpstreamModelName = ""
		d.Properties = props
	}
	d.ModelName = publicModel
	d.Data = relay.RedactTaskDataForPublic(d.Data, publicModel, d.TaskID)
}
