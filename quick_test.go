package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// 快速测试 MaxUsersPerRun 功能
func quickTestMaxUsersPerRun() {
	log.Println("🧪 Quick testing MaxUsersPerRun functionality...")

	// 测试配置
	config := Config{
		Redis: RedisConfig{
			URL: "redis://localhost:6379/0",
		},
		MongoDB: MongoDBConfig{
			URL:      "mongodb://localhost:27017",
			Database: "app",
		},
		Migration: MigrationConfig{
			CheckIntervalMinutes: 1,
			ExpireHours:         1,
			BatchSize:           100,
			WorkerCount:         8,
			MaxConcurrency:      100,
			MaxUsersPerRun:      80,
		},
	}

	// 创建 Redis 客户端
	opt, err := redis.ParseURL(config.Redis.URL)
	if err != nil {
		log.Fatalf("Failed to parse Redis URL: %v", err)
	}
	redisClient := redis.NewClient(opt)
	defer redisClient.Close()

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	// 生成少量测试数据，确保都是过期的
	testUserCount := 100
	log.Printf("Generating %d expired test users...", testUserCount)

	pipe := redisClient.Pipeline()
	for i := 0; i < testUserCount; i++ {
		userID := fmt.Sprintf("test_user_%d", i)
		// 设置访问时间为2小时前，确保过期
		accessTime := time.Now().Add(-2 * time.Hour).UnixMilli()

		pipe.HSet(ctx, "access", userID, strconv.FormatInt(accessTime, 10))
		pipe.Set(ctx, fmt.Sprintf("profile:%s", userID), fmt.Sprintf(`{"displayName":"User %d","avatarUrl":"avatar_%d","userId":"%s"}`, i, i, userID), 0)
		pipe.Set(ctx, fmt.Sprintf("user:%s", userID), fmt.Sprintf(`{"userId":"%s","data":"test_data_%d"}`, userID, i), 0)
	}

	_, err = pipe.Exec(ctx)
	if err != nil {
		log.Fatalf("Failed to generate test data: %v", err)
	}

	log.Printf("✅ Generated %d expired test users", testUserCount)

	// 测试不同的限制值
	testLimits := []int{0, 5, 10, 20, 50}

	for _, limit := range testLimits {
		log.Printf("\n📋 Testing MaxUsersPerRun = %d", limit)

		// 创建测试配置
		testConfig := config
		testConfig.Migration.MaxUsersPerRun = limit

		// 创建迁移器
		migration := NewMigration(testConfig)

		// 连接数据库
		if err := migration.Connect(); err != nil {
			log.Printf("Failed to connect: %v", err)
			continue
		}

		// 执行迁移
		startTime := time.Now()
		err := migration.MigrateExpiredUsers()
		duration := time.Since(startTime)

		if err != nil {
			log.Printf("Migration failed: %v", err)
			migration.Close()
			continue
		}

		// 获取统计信息
		stats := migration.GetStats()

		// 输出测试结果
		log.Printf("  ✅ Results for limit %d:", limit)
		log.Printf("    - Duration: %v", duration)
		log.Printf("    - Users Migrated: %d", stats.TotalMigrated)
		log.Printf("    - Errors: %d", stats.TotalErrors)
		if stats.TotalMigrated > 0 {
			log.Printf("    - Throughput: %.2f users/second", float64(stats.TotalMigrated)/duration.Seconds())
		}

		// 验证限制是否生效
		if limit > 0 && int(stats.TotalMigrated) > limit {
			log.Printf("    ⚠️  WARNING: Migrated %d users but limit was %d", stats.TotalMigrated, limit)
		} else if limit > 0 && int(stats.TotalMigrated) == limit {
			log.Printf("    ✅ Limit correctly enforced: exactly %d users migrated", limit)
		} else if limit == 0 {
			log.Printf("    ✅ No limit: %d users migrated", stats.TotalMigrated)
		} else if limit > 0 && int(stats.TotalMigrated) < limit {
			log.Printf("    ✅ Limit respected: %d users migrated (less than limit %d)", stats.TotalMigrated, limit)
		}

		migration.Close()

		// 等待一下再进行下一个测试
		time.Sleep(1 * time.Second)
	}

	log.Println("\n🎯 MaxUsersPerRun testing completed!")
}

func main() {
	quickTestMaxUsersPerRun()
}
