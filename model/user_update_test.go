package model

import (
	"errors"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type legacyUserForExternalIDMigration struct {
	Id       int `gorm:"primaryKey"`
	Username string
	Password string `gorm:"not null"`
}

func (legacyUserForExternalIDMigration) TableName() string {
	return "users"
}

type userExternalIDMigrationTarget struct {
	Id         int    `gorm:"primaryKey"`
	ExternalId string `gorm:"type:varchar(36);column:external_id"`
}

func (userExternalIDMigrationTarget) TableName() string {
	return "users"
}

func TestUserExternalIDGeneratedAndImmutable(t *testing.T) {
	setupUserUpdateTestState(t)

	requestedExternalID := userExternalIDPrefix + strings.Repeat("b", 32)
	first := User{Username: "external-id-first", Password: "password", ExternalId: requestedExternalID, AffCode: "external-id-first"}
	second := User{Username: "external-id-second", Password: "password", AffCode: "external-id-second"}
	require.NoError(t, DB.Create(&first).Error)
	require.NoError(t, DB.Create(&second).Error)

	for _, externalID := range []string{first.ExternalId, second.ExternalId} {
		assert.True(t, strings.HasPrefix(externalID, userExternalIDPrefix))
		assert.Len(t, externalID, 36)
	}
	assert.NotEqual(t, requestedExternalID, first.ExternalId)
	assert.NotEqual(t, first.ExternalId, second.ExternalId)

	originalExternalID := first.ExternalId
	first.ExternalId = userExternalIDPrefix + strings.Repeat("f", 32)
	first.DisplayName = "updated"
	require.NoError(t, first.Update(false))
	assert.Equal(t, originalExternalID, first.ExternalId)

	err := DB.Model(&User{}).Where("id = ?", second.Id).UpdateColumn("external_id", originalExternalID).Error
	require.Error(t, err)
}

func TestBackfillUserExternalIDsIncludesSoftDeletedUsers(t *testing.T) {
	setupUserUpdateTestState(t)

	preservedID := userExternalIDPrefix + strings.Repeat("a", 32)
	preserved := User{Username: "external-id-preserved", Password: "password", ExternalId: preservedID, AffCode: "external-id-preserved"}
	activeMissing := User{Username: "external-id-active-missing", Password: "password", AffCode: "external-id-active"}
	deletedMissing := User{Username: "external-id-deleted-missing", Password: "password", AffCode: "external-id-deleted"}
	require.NoError(t, DB.Create(&preserved).Error)
	require.NoError(t, DB.Create(&activeMissing).Error)
	require.NoError(t, DB.Create(&deletedMissing).Error)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", preserved.Id).UpdateColumn("external_id", preservedID).Error)
	require.NoError(t, DB.Model(&User{}).Where("id IN ?", []int{activeMissing.Id, deletedMissing.Id}).UpdateColumn("external_id", nil).Error)
	require.NoError(t, DB.Delete(&deletedMissing).Error)

	require.NoError(t, backfillUserExternalIDs(DB))

	var users []User
	require.NoError(t, DB.Unscoped().Where("id IN ?", []int{preserved.Id, activeMissing.Id, deletedMissing.Id}).Order("id ASC").Find(&users).Error)
	require.Len(t, users, 3)
	assert.Equal(t, preservedID, users[0].ExternalId)
	assert.True(t, strings.HasPrefix(users[1].ExternalId, userExternalIDPrefix))
	assert.True(t, strings.HasPrefix(users[2].ExternalId, userExternalIDPrefix))
	assert.NotEqual(t, users[1].ExternalId, users[2].ExternalId)
}

func TestUserExternalIDMigrationBackfillsLegacyRows(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:user-external-id-migration?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&legacyUserForExternalIDMigration{}))
	require.NoError(t, db.Create(&legacyUserForExternalIDMigration{Username: "legacy-a", Password: "password"}).Error)
	require.NoError(t, db.Create(&legacyUserForExternalIDMigration{Username: "legacy-b", Password: "password"}).Error)

	require.NoError(t, db.AutoMigrate(&userExternalIDMigrationTarget{}))
	require.NoError(t, ensureUserExternalIDs(db))
	assert.True(t, db.Migrator().HasIndex(&User{}, userExternalIDUniqueIndexName))

	var users []User
	require.NoError(t, db.Unscoped().Order("id ASC").Find(&users).Error)
	require.Len(t, users, 2)
	for _, user := range users {
		assert.True(t, strings.HasPrefix(user.ExternalId, userExternalIDPrefix))
		assert.Len(t, user.ExternalId, 36)
	}
	assert.NotEqual(t, users[0].ExternalId, users[1].ExternalId)
}

func setupUserUpdateTestState(t *testing.T) {
	t.Helper()
	truncateTables(t)
	require.NoError(t, DB.Exec("DELETE FROM users").Error)
	require.NoError(t, ensureUserExternalIDs(DB))

	oldRedisEnabled := common.RedisEnabled
	oldBatchUpdateEnabled := common.BatchUpdateEnabled
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	t.Cleanup(func() {
		common.RedisEnabled = oldRedisEnabled
		common.BatchUpdateEnabled = oldBatchUpdateEnabled
	})
}

func TestUserUpdateDoesNotOverwriteAccountingFields(t *testing.T) {
	setupUserUpdateTestState(t)

	user := User{
		Id:           1,
		Username:     "quota-race-user",
		Password:     "password",
		DisplayName:  "before",
		Status:       common.UserStatusEnabled,
		Quota:        1000,
		UsedQuota:    20,
		RequestCount: 3,
	}
	require.NoError(t, DB.Create(&user).Error)

	staleUser, err := GetUserById(user.Id, true)
	require.NoError(t, err)

	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Updates(map[string]interface{}{
		"quota":         gorm.Expr("quota - ?", 400),
		"used_quota":    gorm.Expr("used_quota + ?", 400),
		"request_count": gorm.Expr("request_count + ?", 1),
	}).Error)

	staleUser.DisplayName = "after"
	require.NoError(t, staleUser.Update(false))

	var got User
	require.NoError(t, DB.First(&got, user.Id).Error)
	assert.Equal(t, "after", got.DisplayName)
	assert.Equal(t, 600, got.Quota)
	assert.Equal(t, 420, got.UsedQuota)
	assert.Equal(t, 4, got.RequestCount)
}

func TestUpdateUserSettingOnlyUpdatesSetting(t *testing.T) {
	setupUserUpdateTestState(t)

	user := User{
		Id:           2,
		Username:     "setting-user",
		Password:     "password",
		Status:       common.UserStatusEnabled,
		Quota:        1000,
		UsedQuota:    20,
		RequestCount: 3,
	}
	require.NoError(t, DB.Create(&user).Error)

	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Updates(map[string]interface{}{
		"quota":         gorm.Expr("quota - ?", 250),
		"used_quota":    gorm.Expr("used_quota + ?", 250),
		"request_count": gorm.Expr("request_count + ?", 1),
	}).Error)

	require.NoError(t, UpdateUserSetting(user.Id, dto.UserSetting{Language: "zh"}))

	var got User
	require.NoError(t, DB.First(&got, user.Id).Error)
	assert.Equal(t, 750, got.Quota)
	assert.Equal(t, 270, got.UsedQuota)
	assert.Equal(t, 4, got.RequestCount)
	assert.Equal(t, "zh", got.GetSetting().Language)
}

func TestEnsureEmailAvailableRejectsExistingEmailCaseInsensitive(t *testing.T) {
	setupUserUpdateTestState(t)

	require.NoError(t, DB.Create(&User{
		Username: "existing",
		Password: "old-password",
		Email:    "Taken@Example.com",
		Status:   common.UserStatusEnabled,
	}).Error)

	err := EnsureEmailAvailable(" taken@example.COM ", 0)
	require.ErrorIs(t, err, ErrEmailAlreadyTaken)

	user, err := GetUniqueUserByEmail("TAKEN@example.com")
	require.NoError(t, err)
	assert.Equal(t, "existing", user.Username)

	require.NoError(t, EnsureEmailAvailable("taken@example.com", user.Id))
}

func TestInsertRejectsDuplicateEmailWithoutUniqueIndex(t *testing.T) {
	setupUserUpdateTestState(t)

	require.NoError(t, DB.Create(&User{
		Username: "existing",
		Password: "old-password",
		Email:    "taken@example.com",
		Status:   common.UserStatusEnabled,
	}).Error)

	user := &User{
		Username: "oauth-user",
		Email:    "TAKEN@example.com",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
	}

	err := user.Insert(0)
	require.ErrorIs(t, err, ErrEmailAlreadyTaken)

	var count int64
	require.NoError(t, DB.Model(&User{}).Where("username = ?", "oauth-user").Count(&count).Error)
	assert.Zero(t, count)
}

func TestInsertKeepsBlankPasswordForPasswordlessUser(t *testing.T) {
	setupUserUpdateTestState(t)

	user := &User{
		Username: "passwordless-user",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
	}

	require.NoError(t, user.Insert(0))

	var stored User
	require.NoError(t, DB.Where("username = ?", user.Username).First(&stored).Error)
	assert.Empty(t, stored.Password)
}

func TestValidateAndFillRejectsPasswordlessUser(t *testing.T) {
	setupUserUpdateTestState(t)

	require.NoError(t, DB.Create(&User{
		Username: "passwordless-user",
		Password: "",
		Status:   common.UserStatusEnabled,
	}).Error)

	loginUser := User{
		Username: "passwordless-user",
		Password: "NewPassword123",
	}
	err := loginUser.ValidateAndFill()
	require.ErrorIs(t, err, ErrInvalidCredentials)

	var stored User
	require.NoError(t, DB.Where("username = ?", "passwordless-user").First(&stored).Error)
	assert.Empty(t, stored.Password)
}

func TestResetUserPasswordByEmailRequiresSingleActiveMatch(t *testing.T) {
	setupUserUpdateTestState(t)

	require.NoError(t, DB.Create(&User{
		Username: "duplicate-1",
		Password: "old-1",
		Email:    "legacy@example.com",
		AffCode:  "dupe1",
		Status:   common.UserStatusEnabled,
	}).Error)
	require.NoError(t, DB.Create(&User{
		Username: "duplicate-2",
		Password: "old-2",
		Email:    "LEGACY@example.com",
		AffCode:  "dupe2",
		Status:   common.UserStatusEnabled,
	}).Error)

	err := ResetUserPasswordByEmail("legacy@example.com", "NewPassword123")
	require.ErrorIs(t, err, ErrEmailAmbiguous)

	var duplicates []User
	require.NoError(t, DB.Where("LOWER(email) = ?", "legacy@example.com").Order("username asc").Find(&duplicates).Error)
	require.Len(t, duplicates, 2)
	assert.Equal(t, "old-1", duplicates[0].Password)
	assert.Equal(t, "old-2", duplicates[1].Password)

	require.NoError(t, DB.Create(&User{
		Username: "unique",
		Password: "old",
		Email:    "unique@example.com",
		AffCode:  "unique",
		Status:   common.UserStatusEnabled,
	}).Error)

	require.NoError(t, ResetUserPasswordByEmail("UNIQUE@example.com", "NewPassword123"))

	var unique User
	require.NoError(t, DB.Where("username = ?", "unique").First(&unique).Error)
	assert.True(t, common.ValidatePasswordAndHash("NewPassword123", unique.Password))

	err = ResetUserPasswordByEmail("missing@example.com", "NewPassword123")
	require.True(t, errors.Is(err, ErrEmailNotFound))
}
