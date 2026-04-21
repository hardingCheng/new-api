package controller

import (
	"fmt"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
)

// UpdateTaskBulk 薄入口，实际轮询逻辑在 service 层
func UpdateTaskBulk() {
	service.TaskPollingLoop()
}

func GetTaskDetail(c *gin.Context) {
	taskId := c.Query("task_id")
	if taskId == "" {
		common.ApiErrorMsg(c, "task_id is required")
		return
	}
	task, exist, err := model.GetByOnlyTaskId(taskId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !exist {
		common.ApiErrorMsg(c, "task not found")
		return
	}
	common.ApiSuccess(c, relay.TaskModel2Dto(task))
}

func GetUserTaskDetail(c *gin.Context) {
	taskId := c.Query("task_id")
	if taskId == "" {
		common.ApiErrorMsg(c, "task_id is required")
		return
	}
	userId := c.GetInt("id")
	task, exist, err := model.GetByTaskId(userId, taskId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !exist {
		common.ApiErrorMsg(c, "task not found")
		return
	}
	common.ApiSuccess(c, relay.TaskModel2Dto(task))
}

func GetAllTask(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)

	queryParams := parseTaskQueryParams(c)

	items := model.TaskGetAllTasks(pageInfo.GetStartIdx(), pageInfo.GetPageSize(), queryParams)
	total := model.TaskCountAllTasks(queryParams)
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(tasksToDto(items, true))
	common.ApiSuccess(c, pageInfo)
}

func ExportAllTask(c *gin.Context) {
	queryParams := parseTaskQueryParams(c)
	// 导出接口不分页，设置一个较大的 limit
	items := model.TaskGetAllTasks(0, 200000, queryParams)
	if len(items) == 0 {
		common.ApiErrorMsg(c, "no task records to export")
		return
	}
	taskDtos := tasksToDto(items, true)
	if err := exportTaskAsXlsx(c, taskDtos, "all"); err != nil {
		common.ApiError(c, err)
		return
	}
}

func GetUserTask(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)

	userId := c.GetInt("id")
	queryParams := parseTaskQueryParams(c)

	items := model.TaskGetAllUserTask(userId, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), queryParams)
	total := model.TaskCountAllUserTask(userId, queryParams)
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(tasksToDto(items, false))
	common.ApiSuccess(c, pageInfo)
}

func ExportUserTask(c *gin.Context) {
	userId := c.GetInt("id")
	queryParams := parseTaskQueryParams(c)
	items := model.TaskGetAllUserTask(userId, 0, 200000, queryParams)
	if len(items) == 0 {
		common.ApiErrorMsg(c, "no task records to export")
		return
	}
	taskDtos := tasksToDto(items, false)
	if err := exportTaskAsXlsx(c, taskDtos, "self"); err != nil {
		common.ApiError(c, err)
		return
	}
}

func tasksToDto(tasks []*model.Task, fillUser bool) []*dto.TaskDto {
	var userIdMap map[int]*model.UserBase
	channelIds := types.NewSet[int]()
	for _, task := range tasks {
		if task.ChannelId > 0 {
			channelIds.Add(task.ChannelId)
		}
	}
	channelNameMap := getTaskChannelNameMap(channelIds.Items())
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
	result := make([]*dto.TaskDto, len(tasks))
	for i, task := range tasks {
		if fillUser {
			if user, ok := userIdMap[task.UserId]; ok {
				task.Username = user.Username
			}
		}
		taskDto := relay.TaskModel2Dto(task)
		if task.ChannelId > 0 {
			taskDto.ChannelName = channelNameMap[task.ChannelId]
		}
		result[i] = taskDto
	}
	return result
}

func getTaskChannelNameMap(channelIds []int) map[int]string {
	channelNameMap := make(map[int]string, len(channelIds))
	if len(channelIds) == 0 {
		return channelNameMap
	}
	if common.MemoryCacheEnabled {
		for _, channelId := range channelIds {
			cacheChannel, err := model.CacheGetChannel(channelId)
			if err == nil && cacheChannel != nil {
				channelNameMap[channelId] = cacheChannel.Name
			}
		}
		return channelNameMap
	}

	var channels []struct {
		Id   int    `gorm:"column:id"`
		Name string `gorm:"column:name"`
	}
	if err := model.DB.Table("channels").Select("id, name").Where("id IN ?", channelIds).Find(&channels).Error; err != nil {
		return channelNameMap
	}
	for _, channel := range channels {
		channelNameMap[channel.Id] = channel.Name
	}
	return channelNameMap
}

func parseTaskQueryParams(c *gin.Context) model.SyncTaskQueryParams {
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	params := model.SyncTaskQueryParams{
		Platform:          constant.TaskPlatform(c.Query("platform")),
		TaskID:            c.Query("task_id"),
		Status:            c.Query("status"),
		Action:            c.Query("action"),
		StartTimestamp:    startTimestamp,
		EndTimestamp:      endTimestamp,
		ChannelID:         c.Query("channel_id"),
		ModelName:         c.Query("model_name"),
		HasVideoReference: c.Query("has_video_reference"),
	}
	// Resolve username to user IDs
	if username := c.Query("username"); username != "" {
		params.Username = username
		var userIDs []int
		model.DB.Model(&model.User{}).Where("username LIKE ?", "%"+username+"%").Pluck("id", &userIDs)
		if len(userIDs) > 0 {
			params.UserIDs = userIDs
		} else {
			// No matching users — use impossible ID to ensure empty result
			params.UserIDs = []int{-1}
		}
	}
	return params
}

func exportTaskAsXlsx(c *gin.Context, items []*dto.TaskDto, scope string) error {
	f := excelize.NewFile()
	sheetName := "tasks"
	f.SetSheetName("Sheet1", sheetName)

	headers := []string{
		"ID", "任务ID", "平台", "用户ID", "用户名", "分组", "渠道ID", "渠道名称", "消耗金额",
		"类型", "状态", "失败原因", "结果URL", "进度", "模型名称", "视频时长(秒)", "视频参考", "退款金额",
		"提交时间", "开始时间", "结束时间", "创建时间", "更新时间",
	}

	for i, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		if err := f.SetCellValue(sheetName, cell, header); err != nil {
			return err
		}
	}

	for idx, item := range items {
		row := idx + 2
		values := []any{
			item.ID,
			item.TaskID,
			item.Platform,
			item.UserId,
			item.Username,
			item.Group,
			item.ChannelId,
			item.ChannelName,
			float64(item.Quota) / common.QuotaPerUnit,
			item.Action,
			item.Status,
			item.FailReason,
			item.ResultURL,
			item.Progress,
			item.ModelName,
			item.VideoDuration,
			item.HasVideoReference,
			float64(item.RefundQuota) / common.QuotaPerUnit,
			formatUnix(item.SubmitTime),
			formatUnix(item.StartTime),
			formatUnix(item.FinishTime),
			formatUnix(item.CreatedAt),
			formatUnix(item.UpdatedAt),
		}
		for col, value := range values {
			cell, _ := excelize.CoordinatesToCellName(col+1, row)
			if err := f.SetCellValue(sheetName, cell, value); err != nil {
				return err
			}
		}
	}

	filename := fmt.Sprintf("task_export_%s_%s.xlsx", scope, time.Now().Format("20060102_150405"))
	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Header("Content-Transfer-Encoding", "binary")
	c.Header("Expires", "0")
	c.Header("Cache-Control", "must-revalidate")
	c.Header("Pragma", "public")

	if err := f.Write(c.Writer); err != nil {
		return err
	}
	return nil
}

func formatUnix(ts int64) string {
	if ts <= 0 {
		return ""
	}
	return time.Unix(ts, 0).Format("2006-01-02 15:04:05")
}
