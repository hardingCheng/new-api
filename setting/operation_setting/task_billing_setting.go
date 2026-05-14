package operation_setting

import (
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

const (
	SeedanceReferenceVideoBillingModeLegacy       = "legacy"
	SeedanceReferenceVideoBillingModeDurationOnly = "duration"
)

type TaskBillingSetting struct {
	SeedanceReferenceVideoMode string `json:"seedance_reference_video_mode"`
}

var taskBillingSetting = TaskBillingSetting{
	SeedanceReferenceVideoMode: SeedanceReferenceVideoBillingModeLegacy,
}

func init() {
	config.GlobalConfig.Register("task_billing_setting", &taskBillingSetting)
}

func GetTaskBillingSetting() *TaskBillingSetting {
	return &taskBillingSetting
}

func NormalizeSeedanceReferenceVideoBillingMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case SeedanceReferenceVideoBillingModeDurationOnly:
		return SeedanceReferenceVideoBillingModeDurationOnly
	default:
		return SeedanceReferenceVideoBillingModeLegacy
	}
}

func GetSeedanceReferenceVideoBillingMode() string {
	return NormalizeSeedanceReferenceVideoBillingMode(taskBillingSetting.SeedanceReferenceVideoMode)
}
