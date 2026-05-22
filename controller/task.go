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

// UpdateTaskBulk 薄入口，实际轮询逻辑在 service 层
func UpdateTaskBulk() {
	service.TaskPollingLoop()
}

func GetAllTask(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)

	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	// 解析其他查询参数
	referenceMode, referenceText := parseTaskReferenceQuery(c.Query("reference"))
	queryParams := model.SyncTaskQueryParams{
		Platform:       constant.TaskPlatform(c.Query("platform")),
		TaskID:         c.Query("task_id"),
		UserID:         c.Query("user_id"),
		UserIDs:        parseTaskUserIDs(c.Query("user_ids")),
		Status:         c.Query("status"),
		Action:         c.Query("action"),
		ModelName:      c.Query("model_name"),
		ModelNames:     parseTaskStringList(c.Query("model_names")),
		Reference:      referenceText,
		ReferenceMode:  referenceMode,
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
		ChannelID:      c.Query("channel_id"),
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
		Status:         c.Query("status"),
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

func parseTaskStringList(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '，' || r == '\n' || r == '\r' || r == '\t'
	})
	values := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		values = append(values, item)
	}
	return values
}

func parseTaskUserIDs(value string) []int {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '，' || r == '\n' || r == '\r' || r == '\t' || r == ' '
	})
	userIDs := make([]int, 0, len(parts))
	seen := make(map[int]struct{}, len(parts))
	for _, part := range parts {
		userID, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || userID <= 0 {
			continue
		}
		if _, ok := seen[userID]; ok {
			continue
		}
		seen[userID] = struct{}{}
		userIDs = append(userIDs, userID)
	}
	return userIDs
}

func parseTaskReferenceQuery(value string) (mode string, text string) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "with", "has", "yes":
		return "with", ""
	case "without", "none", "no":
		return "without", ""
	default:
		return "", value
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
		if fillUser {
			result[i] = relay.TaskModel2AdminDto(task)
		} else {
			result[i] = relay.TaskModel2Dto(task)
		}
	}
	return result
}
