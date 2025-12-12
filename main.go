package main

import (
	"log"
	"time"
)

// Start 开始迁移任务
func (m *Migration) Start() {
	log.Println("🚀 Starting Migration service...")
	log.Printf("📊 Configuration:")
	log.Printf("  - Worker Count: %d", cap(m.workerPool))
	log.Printf("  - Check Interval: %d minutes", m.config.Migration.CheckIntervalMinutes)
	log.Printf("  - Expire Hours: %d", m.config.Migration.ExpireHours)
	log.Printf("  - Batch Size: %d", m.config.Migration.BatchSize)

	checkInterval := time.Duration(m.config.Migration.CheckIntervalMinutes) * time.Minute
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	// 立即执行一次
	log.Println("🔄 Running initial migration...")
	if err := m.MigrateExpiredUsers(); err != nil {
		log.Printf("❌ Initial migration failed: %v", err)
	}

	// 定时执行
	for range ticker.C {
		log.Println("🔄 Running scheduled migration...")
		if err := m.MigrateExpiredUsers(); err != nil {
			log.Printf("❌ Migration failed: %v", err)
		}

		// 输出统计信息
		stats := m.GetStats()
		log.Printf("📈 Statistics - Total Migrated: %d, Total Errors: %d, Last Run: %v",
			stats.TotalMigrated, stats.TotalErrors, stats.LastRunTime.Format("2006-01-02 15:04:05"))
	}
}


func main() {
	log.Println("🚀 Starting Migration Service...")

	// 加载配置
	config, err := loadConfig("config.json")
	if err != nil {
		log.Fatalf("❌ Failed to load config: %v", err)
	}

	log.Printf("📋 Configuration loaded:")
	log.Printf("  Redis URL: %s", config.Redis.URL)
	log.Printf("  MongoDB URL: %s", config.MongoDB.URL)
	log.Printf("  Check Interval: %d minutes", config.Migration.CheckIntervalMinutes)
	log.Printf("  Expire Hours: %d", config.Migration.ExpireHours)
	log.Printf("  Worker Count: %d", config.Migration.WorkerCount)
	log.Printf("  Batch Size: %d", config.Migration.BatchSize)

	// 创建迁移器
	migration := NewMigration(config)

	// 连接数据库
	log.Println("🔌 Connecting to databases...")
	if err := migration.Connect(); err != nil {
		log.Fatalf("❌ Failed to connect: %v", err)
	}
	defer migration.Close()

	log.Println("✅ All connections established successfully!")

	// 开始迁移任务
	migration.Start()
}