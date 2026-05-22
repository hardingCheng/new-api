package controller

import (
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

const (
	taskExportDefaultLimit = 10000
	taskExportMaxLimit     = 50000
)

// UpdateTaskBulk 薄入口，实际轮询逻辑在 service 层
func UpdateTaskBulk() {
	service.TaskPollingLoop()
}

func GetAllTask(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)

	queryParams := parseAdminTaskQuery(c)

	items := model.TaskGetAllTasks(pageInfo.GetStartIdx(), pageInfo.GetPageSize(), queryParams)
	total := model.TaskCountAllTasks(queryParams)
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(tasksToDto(items, true))
	common.ApiSuccess(c, pageInfo)
}

func ExportAllTask(c *gin.Context) {
	queryParams := parseAdminTaskQuery(c)
	limit := parseTaskExportLimit(c)
	items := model.TaskGetAllTasks(0, limit, queryParams)
	exportTasksCSV(c, tasksToDto(items, true))
}

func parseAdminTaskQuery(c *gin.Context) model.SyncTaskQueryParams {
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	// 解析其他查询参数
	referenceMode, referenceText := parseTaskReferenceQuery(c.Query("reference"))
	return model.SyncTaskQueryParams{
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

func parseTaskExportLimit(c *gin.Context) int {
	limit := taskExportDefaultLimit
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	} else if raw := strings.TrimSpace(c.Query("page_size")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	if limit <= 0 {
		return taskExportDefaultLimit
	}
	if limit > taskExportMaxLimit {
		return taskExportMaxLimit
	}
	return limit
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
	var channelNameMap map[int]string
	if fillUser {
		userIdMap = make(map[int]*model.UserBase)
		userIds := types.NewSet[int]()
		channelIds := types.NewSet[int]()
		for _, task := range tasks {
			userIds.Add(task.UserId)
			if task.ChannelId > 0 {
				channelIds.Add(task.ChannelId)
			}
		}
		for _, userId := range userIds.Items() {
			cacheUser, err := model.GetUserCache(userId)
			if err == nil {
				userIdMap[userId] = cacheUser
			}
		}
		channelNameMap = make(map[int]string)
		if ids := channelIds.Items(); len(ids) > 0 {
			var channels []model.Channel
			if err := model.DB.Select("id, name").Where("id in ?", ids).Find(&channels).Error; err == nil {
				for _, channel := range channels {
					channelNameMap[channel.Id] = channel.Name
				}
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
			result[i].ChannelName = channelNameMap[task.ChannelId]
		} else {
			result[i] = relay.TaskModel2Dto(task)
		}
	}
	return result
}

func exportTasksCSV(c *gin.Context, tasks []*dto.TaskDto) {
	filename := fmt.Sprintf("task-bills-%s.csv", time.Now().Format("20060102-150405"))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Status(200)

	_, _ = c.Writer.Write([]byte{0xEF, 0xBB, 0xBF})
	writer := csv.NewWriter(c.Writer)
	defer writer.Flush()

	_ = writer.Write([]string{
		"任务ID",
		"提交时间",
		"结束时间",
		"用户ID",
		"用户名",
		"渠道编号",
		"渠道名称",
		"平台",
		"调用模型",
		"任务类型",
		"任务状态",
		"预扣金额",
		"消费金额",
		"退款金额",
		"视频时长",
		"视频参考",
	})

	for _, task := range tasks {
		_ = writer.Write([]string{
			task.TaskID,
			formatTaskExportTime(task.SubmitTime),
			formatTaskExportTime(task.FinishTime),
			strconv.Itoa(task.UserId),
			task.Username,
			strconv.Itoa(task.ChannelId),
			task.ChannelName,
			formatTaskExportPlatform(task.Platform),
			task.ModelName,
			task.Action,
			task.Status,
			formatTaskExportQuota(task.Quota),
			formatTaskExportQuotaPtr(task.ConsumedQuota),
			formatTaskExportQuotaPtr(task.RefundQuota),
			task.VideoSeconds,
			formatTaskExportBoolPtr(task.HasVideoReference),
		})
	}
}

func formatTaskExportTime(timestamp int64) string {
	if timestamp <= 0 {
		return ""
	}
	return time.Unix(timestamp, 0).Format("2006-01-02 15:04:05")
}

func formatTaskExportQuota(quota int) string {
	amountUSD := float64(quota) / common.QuotaPerUnit
	switch operation_setting.GetQuotaDisplayType() {
	case operation_setting.QuotaDisplayTypeCNY:
		return fmt.Sprintf("%.6f", amountUSD*operation_setting.USDExchangeRate)
	case operation_setting.QuotaDisplayTypeCustom:
		setting := operation_setting.GetGeneralSetting()
		rate := setting.CustomCurrencyExchangeRate
		if rate <= 0 {
			rate = 1
		}
		return fmt.Sprintf("%.6f", amountUSD*rate)
	default:
		return fmt.Sprintf("%.6f", amountUSD)
	}
}

func formatTaskExportQuotaPtr(value *int) string {
	if value == nil {
		return ""
	}
	return formatTaskExportQuota(*value)
}

func formatTaskExportBoolPtr(value *bool) string {
	if value == nil {
		return ""
	}
	if *value {
		return "是"
	}
	return "否"
}

func formatTaskExportPlatform(platform string) string {
	if platformID, err := strconv.Atoi(platform); err == nil {
		return constant.GetChannelTypeName(platformID)
	}
	return platform
}
