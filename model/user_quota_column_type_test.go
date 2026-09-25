package model

import (
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/schema"
)

// 启动检查 ensureUserQuotaColumns 要求额度列是 64 位；模型标注若仍是 int，
// MySQL 上 AutoMigrate 会把手工改好的 bigint 列改回 int，下次重启即拒绝启动。
func TestUserQuotaColumnsDeclareBigint(t *testing.T) {
	parsed, err := schema.Parse(&User{}, &sync.Map{}, schema.NamingStrategy{})
	require.NoError(t, err)
	for _, column := range userQuotaColumns {
		field := parsed.LookUpField(column)
		require.NotNil(t, field, column)
		assert.Equal(t, "bigint", strings.ToLower(field.TagSettings["TYPE"]), column)
	}
}
