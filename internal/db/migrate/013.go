package migrate

import (
	"fmt"

	"gorm.io/gorm"
)

func init() {
	RegisterAfterAutoMigration(Migration{
		Version: 13,
		Up:      backfillChannelResponseHeaders,
	})
}

// backfillChannelResponseHeaders 把 channels.response_headers 列的 NULL 改写为空数组。
// 该列由 AutoMigrate 新增, 既有渠道的行为 NULL, JSON 序列化成 null 会让前端读取 .length 时崩溃;
// 改写后由 GORM 的 json 序列化器按空数组读写, 与"未配置响应头"的语义一致。已是数组的行跳过, 中途失败可重跑。
func backfillChannelResponseHeaders(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}
	if !db.Migrator().HasTable("channels") || !hasPhysicalColumn(db, "channels", "response_headers") {
		return nil
	}

	if err := db.Exec("UPDATE channels SET response_headers = '[]' WHERE response_headers IS NULL").Error; err != nil {
		return fmt.Errorf("failed to backfill response_headers: %w", err)
	}
	return nil
}
