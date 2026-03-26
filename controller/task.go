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
		result[i] = relay.TaskModel2Dto(task)
	}
	return result
}

func parseTaskQueryParams(c *gin.Context) model.SyncTaskQueryParams {
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	return model.SyncTaskQueryParams{
		Platform:       constant.TaskPlatform(c.Query("platform")),
		TaskID:         c.Query("task_id"),
		Status:         c.Query("status"),
		Action:         c.Query("action"),
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
		ChannelID:      c.Query("channel_id"),
	}
}

func exportTaskAsXlsx(c *gin.Context, items []*dto.TaskDto, scope string) error {
	f := excelize.NewFile()
	sheetName := "tasks"
	f.SetSheetName("Sheet1", sheetName)

	headers := []string{
		"id", "task_id", "platform", "user_id", "username", "group", "channel_id", "quota",
		"action", "status", "fail_reason", "result_url", "progress", "submit_time", "start_time", "finish_time",
		"created_at", "updated_at",
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
			item.Quota,
			item.Action,
			item.Status,
			item.FailReason,
			item.ResultURL,
			item.Progress,
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
