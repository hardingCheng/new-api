package model

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/require"
)

func TestHandleConfigUpdateTaskBillingSetting(t *testing.T) {
	settings := operation_setting.GetTaskBillingSetting()
	originalMode := settings.SeedanceReferenceVideoMode
	t.Cleanup(func() {
		settings.SeedanceReferenceVideoMode = originalMode
	})

	handled := handleConfigUpdate(
		"task_billing_setting.seedance_reference_video_mode",
		operation_setting.SeedanceReferenceVideoBillingModeDurationOnly,
	)

	require.True(t, handled)
	require.Equal(
		t,
		operation_setting.SeedanceReferenceVideoBillingModeDurationOnly,
		operation_setting.GetSeedanceReferenceVideoBillingMode(),
	)
}
