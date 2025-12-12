package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// PerformanceTest 性能测试工具
type PerformanceTest struct {
	redisClient *redis.Client
}

// NewPerformanceTest 创建性能测试实例
func NewPerformanceTest(redisURL string) (*PerformanceTest, error) {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}

	client := redis.NewClient(opt)

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	return &PerformanceTest{redisClient: client}, nil
}

// GenerateTestData 生成测试数据
func (pt *PerformanceTest) GenerateTestData(userCount int) error {
	ctx := context.Background()

	log.Printf("Generating %d test users...", userCount)

	// 生成访问时间数据
	pipe := pt.redisClient.Pipeline()
	for i := 0; i < userCount; i++ {
		userID := fmt.Sprintf("test_user_%d", i)
		accessTime := time.Now().Add(-time.Duration(i%24) * time.Hour).UnixMilli()

		pipe.HSet(ctx, "access", userID, strconv.FormatInt(accessTime, 10))
		pipe.Set(ctx, fmt.Sprintf("profile:%s", userID), fmt.Sprintf(`{"displayName":"User %d","avatarUrl":"avatar_%d","userId":"%s"}`, i, i, userID), 0)
		pipe.Set(ctx, fmt.Sprintf("user:%s", userID), fmt.Sprintf(`{"userId":"%s","data":"test_data_%d"}`, userID, i), 0)
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		return err
	}

	log.Printf("✅ Generated %d test users", userCount)
	return nil
}

// CleanTestData 清理测试数据
func (pt *PerformanceTest) CleanTestData() error {
	ctx := context.Background()

	log.Println("Cleaning test data...")

	// 获取所有测试用户
	accessData, err := pt.redisClient.HGetAll(ctx, "access").Result()
	if err != nil {
		return err
	}

	var testUsers []string
	for userID := range accessData {
		if len(userID) > 10 && userID[:10] == "test_user_" {
			testUsers = append(testUsers, userID)
		}
	}

	// 删除测试数据
	pipe := pt.redisClient.Pipeline()
	for _, userID := range testUsers {
		pipe.HDel(ctx, "access", userID)
		pipe.Del(ctx, fmt.Sprintf("profile:%s", userID))
		pipe.Del(ctx, fmt.Sprintf("user:%s", userID))
	}

	_, err = pipe.Exec(ctx)
	if err != nil {
		return err
	}

	log.Printf("✅ Cleaned %d test users", len(testUsers))
	return nil
}

// BenchmarkMigration 性能基准测试
func (pt *PerformanceTest) BenchmarkMigration(config Config) error {
	log.Println("🚀 Starting performance benchmark...")

	// 创建迁移器
	migration := NewMigration(config)

	// 连接数据库
	if err := migration.Connect(); err != nil {
		return err
	}
	defer migration.Close()

	// 执行迁移
	startTime := time.Now()
	err := migration.MigrateExpiredUsers()
	duration := time.Since(startTime)

	if err != nil {
		return err
	}

	// 输出性能统计
	stats := migration.GetStats()
	log.Printf("📊 Performance Results:")
	log.Printf("  - Duration: %v", duration)
	log.Printf("  - Users Migrated: %d", stats.TotalMigrated)
	log.Printf("  - Errors: %d", stats.TotalErrors)
	log.Printf("  - Throughput: %.2f users/second", float64(stats.TotalMigrated)/duration.Seconds())

	return nil
}

// TestMaxUsersPerRun 测试用户数量限制功能
func (pt *PerformanceTest) TestMaxUsersPerRun(config Config) error {
	log.Println("🧪 Testing MaxUsersPerRun functionality...")

	// 使用配置中的 MaxUsersPerRun 值
	limit := config.Migration.MaxUsersPerRun
	log.Printf("📋 Testing with MaxUsersPerRun = %d", limit)

	round := 1
	totalMigrated := int64(0)

	for {
		log.Printf("\n🔄 Round %d - Testing MaxUsersPerRun = %d", round, limit)

		// 创建迁移器
		migration := NewMigration(config)

		// 连接数据库
		if err := migration.Connect(); err != nil {
			return err
		}

		// 执行迁移
		startTime := time.Now()
		err := migration.MigrateExpiredUsers()
		duration := time.Since(startTime)

		if err != nil {
			migration.Close()
			return err
		}

		// 获取统计信息
		stats := migration.GetStats()
		totalMigrated += stats.TotalMigrated

		// 输出测试结果
		log.Printf("  ✅ Round %d Results:", round)
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
		} else if limit > 0 && int(stats.TotalMigrated) < limit && stats.TotalMigrated > 0 {
			log.Printf("    ✅ Limit respected: %d users migrated (less than limit %d)", stats.TotalMigrated, limit)
		} else if stats.TotalMigrated == 0 {
			log.Printf("    ✅ No expired users found - migration complete")
		}

		migration.Close()

		// 如果没有迁移任何用户，说明没有过期数据了，测试结束
		if stats.TotalMigrated == 0 {
			log.Printf("\n🎯 MaxUsersPerRun testing completed!")
			log.Printf("📊 Total Summary:")
			log.Printf("  - Total Rounds: %d", round)
			log.Printf("  - Total Users Migrated: %d", totalMigrated)
			log.Printf("  - MaxUsersPerRun Limit: %d", limit)
			break
		}

		round++

		// 等待一下再进行下一轮测试
		time.Sleep(60 * time.Second)
	}

	return nil
}

func main() {
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

	// 创建性能测试实例
	pt, err := NewPerformanceTest(config.Redis.URL)
	if err != nil {
		log.Fatalf("Failed to create performance test: %v", err)
	}
	defer pt.redisClient.Close()

	// 生成测试数据
	testUserCount := 10000
	if err := pt.GenerateTestData(testUserCount); err != nil {
		log.Fatalf("Failed to generate test data: %v", err)
	}

	log.Println("⏳ Waiting 60 seconds for data to expire...")
	time.Sleep(60 * time.Second)

	// 1. 执行 MaxUsersPerRun 功能测试
	log.Println("\n" + strings.Repeat("=", 60))
	log.Println("🧪 PHASE 1: MaxUsersPerRun Functionality Test")
	log.Println(strings.Repeat("=", 60))

	if err := pt.TestMaxUsersPerRun(config); err != nil {
		log.Fatalf("MaxUsersPerRun test failed: %v", err)
	}

	// 重新生成测试数据（因为之前的测试已经迁移了一些用户）
	log.Println("\n⏳ Regenerating test data for performance benchmark...")
	if err := pt.GenerateTestData(testUserCount); err != nil {
		log.Fatalf("Failed to regenerate test data: %v", err)
	}

	log.Println("⏳ Waiting 60 seconds for data to expire...")
	time.Sleep(60 * time.Second)

	// 2. 执行性能基准测试
	log.Println("\n" + strings.Repeat("=", 60))
	log.Println("🚀 PHASE 2: Performance Benchmark Test")
	log.Println(strings.Repeat("=", 60))

	if err := pt.BenchmarkMigration(config); err != nil {
		log.Fatalf("Performance benchmark failed: %v", err)
	}

	// 清理测试数据
	log.Println("\n🧹 Cleaning up test data...")
	if err := pt.CleanTestData(); err != nil {
		log.Printf("Warning: Failed to clean test data: %v", err)
	}

	log.Println("\n✅ All benchmark tests completed successfully!")
}
