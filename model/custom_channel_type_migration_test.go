package model

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetCustomChannelTypeMigrationTables(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&Channel{}, &Option{}, &Task{}))
	require.NoError(t, DB.Exec("DELETE FROM channels").Error)
	require.NoError(t, DB.Exec("DELETE FROM tasks").Error)
	require.NoError(t, DB.Where(commonKeyCol+" = ?", customChannelTypeMigrationKey).Delete(&Option{}).Error)
}

func TestMigrateCustomChannelTypesMovesLegacyNumbers(t *testing.T) {
	resetCustomChannelTypeMigrationTables(t)
	for _, ch := range []*Channel{
		{Id: 1, Type: 61, Name: "dmc", Key: "k1"},
		{Id: 2, Type: 62, Name: "tokenmart", Key: "k2"},
		{Id: 3, Type: 63, Name: "mps", Key: "k3"},
		{Id: 4, Type: constant.ChannelTypeSora, Name: "sora", Key: "k4"},
	} {
		require.NoError(t, DB.Create(ch).Error)
	}
	require.NoError(t, DB.Create(&Task{TaskID: "task_a", Platform: "61"}).Error)
	require.NoError(t, DB.Create(&Task{TaskID: "task_b", Platform: "55"}).Error)

	require.NoError(t, migrateCustomChannelTypes(DB))

	types := map[int]int{}
	var channels []Channel
	require.NoError(t, DB.Find(&channels).Error)
	for _, ch := range channels {
		types[ch.Id] = ch.Type
	}
	assert.Equal(t, map[int]int{
		1: constant.ChannelTypeDMC,
		2: constant.ChannelTypeTokenMart,
		3: constant.ChannelTypeTencentMPS,
		4: constant.ChannelTypeSora,
	}, types)

	var taskA, taskB Task
	require.NoError(t, DB.Where("task_id = ?", "task_a").First(&taskA).Error)
	require.NoError(t, DB.Where("task_id = ?", "task_b").First(&taskB).Error)
	assert.Equal(t, constant.TaskPlatform("64"), taskA.Platform)
	assert.Equal(t, constant.TaskPlatform("55"), taskB.Platform)
}

func TestMigrateCustomChannelTypesRunsOnlyOnce(t *testing.T) {
	resetCustomChannelTypeMigrationTables(t)
	require.NoError(t, migrateCustomChannelTypes(DB))

	// 迁移之后 61 是官方 Task Plugin 类型，不能再被挪走
	require.NoError(t, DB.Create(&Channel{Id: 9, Type: constant.ChannelTypeTaskPlugin, Name: "plugin", Key: "k"}).Error)
	require.NoError(t, migrateCustomChannelTypes(DB))

	var ch Channel
	require.NoError(t, DB.First(&ch, 9).Error)
	assert.Equal(t, constant.ChannelTypeTaskPlugin, ch.Type)
}
