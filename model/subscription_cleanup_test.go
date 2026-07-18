package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanupSubscriptionPreConsumeRecordsKeepsPendingRefundState(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&SubscriptionPreConsumeRecord{}))
	t.Cleanup(func() { DB.Exec("DELETE FROM subscription_pre_consume_records") })
	oldTimestamp := common.GetTimestamp() - 8*24*3600
	records := []SubscriptionPreConsumeRecord{
		{RequestId: "old-consumed", UserId: 1, UserSubscriptionId: 1, PreConsumed: 100, Status: "consumed"},
		{RequestId: "old-refunded", UserId: 1, UserSubscriptionId: 1, PreConsumed: 100, Status: "refunded"},
	}
	for index := range records {
		require.NoError(t, DB.Create(&records[index]).Error)
		require.NoError(t, DB.Model(&records[index]).Updates(map[string]interface{}{
			"created_at": oldTimestamp,
			"updated_at": oldTimestamp,
		}).Error)
	}

	deleted, err := CleanupSubscriptionPreConsumeRecords(7 * 24 * 3600)

	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)
	var remaining []SubscriptionPreConsumeRecord
	require.NoError(t, DB.Find(&remaining).Error)
	require.Len(t, remaining, 1)
	assert.Equal(t, "old-consumed", remaining[0].RequestId)
}
