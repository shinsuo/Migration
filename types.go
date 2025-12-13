package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Config 配置结构
type Config struct {
	Redis     RedisConfig     `json:"redis"`
	MongoDB   MongoDBConfig   `json:"mongodb"`
	Migration MigrationConfig `json:"migration"`
}

type RedisConfig struct {
	URL string `json:"url"`
	TLS bool   `json:"tls"`
}

type MongoDBConfig struct {
	URL      string `json:"url"`
	Database string `json:"database"`
	Username string `json:"username"`
	Password string `json:"password"`
	TLSFile  string `json:"tlsCAFile"`
}

type MigrationConfig struct {
	CheckIntervalMinutes int `json:"checkIntervalMinutes"`
	ExpireHours          int `json:"expireHours"`
	BatchSize            int `json:"batchSize"`
	WorkerCount          int `json:"workerCount"`
	MaxConcurrency       int `json:"maxConcurrency"`
	MaxUsersPerRun       int `json:"maxUsersPerRun"`
}

// UserData 用户数据结构
type UserData struct {
	UserID string `bson:"userId" json:"userId"`
	// Profile     Profile   `bson:"profile" json:"profile"`
	CacheString string    `bson:"cacheString" json:"cacheString"`
	AccessTime  time.Time `bson:"accessTime" json:"accessTime"`
	MigratedAt  time.Time `bson:"migratedAt" json:"migratedAt"`
}

// Profile 用户资料结构
type Profile struct {
	DisplayName string `bson:"Name" json:"Name"`
	AvatarUrl   string `bson:"AvatarUrl" json:"AvatarUrl"`
	UserID      string `bson:"userId" json:"userId"`
}

// Migration 迁移器
type Migration struct {
	redisClient *redis.Client
	mongoClient *mongo.Client
	config      Config
	workerPool  chan struct{}
	stats       *MigrationStats
}

// MigrationStats 迁移统计信息
type MigrationStats struct {
	TotalProcessed int64
	TotalMigrated  int64
	TotalErrors    int64
	LastRunTime    time.Time
	mu             sync.RWMutex
}

// UserTask 用户任务
type UserTask struct {
	UserID        string
	AccessTime    time.Time
	AccessTimeStr string
}

// NewMigration 创建新的迁移器
func NewMigration(config Config) *Migration {
	workerCount := config.Migration.WorkerCount
	if workerCount <= 0 {
		workerCount = runtime.NumCPU() * 2 // 默认使用 CPU 核心数的 2 倍
	}

	return &Migration{
		config:     config,
		workerPool: make(chan struct{}, workerCount),
		stats: &MigrationStats{
			TotalProcessed: 0,
			TotalMigrated:  0,
			TotalErrors:    0,
		},
	}
}

func getCustomTLSConfig(caFile string) (*tls.Config, error) {
	tlsConfig := new(tls.Config)
	certs, err := ioutil.ReadFile(caFile)

	if err != nil {
		return tlsConfig, err
	}

	tlsConfig.RootCAs = x509.NewCertPool()
	ok := tlsConfig.RootCAs.AppendCertsFromPEM(certs)

	if !ok {
		return tlsConfig, errors.New("Failed parsing pem file")
	}

	return tlsConfig, nil
}

// Connect 连接数据库
func (m *Migration) Connect() error {
	// 连接 Redis
	redisURL := m.config.Redis.URL
	// 根据 TLS 配置决定使用 rediss:// 还是 redis:// 协议
	if m.config.Redis.TLS {
		// TLS 为 true 时使用 rediss:// 协议
		if strings.HasPrefix(redisURL, "redis://") {
			redisURL = strings.Replace(redisURL, "redis://", "rediss://", 1)
		} else if !strings.HasPrefix(redisURL, "rediss://") {
			// 如果 URL 没有协议前缀，添加 rediss://
			redisURL = "rediss://" + redisURL
		}
	} else {
		// TLS 为 false 时使用 redis:// 协议
		if strings.HasPrefix(redisURL, "rediss://") {
			redisURL = strings.Replace(redisURL, "rediss://", "redis://", 1)
		} else if !strings.HasPrefix(redisURL, "redis://") {
			// 如果 URL 没有协议前缀，添加 redis://
			redisURL = "redis://" + redisURL
		}
	}

	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return fmt.Errorf("failed to parse redis URL: %v", err)
	}
	m.redisClient = redis.NewClient(opt)

	// 测试 Redis 连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := m.redisClient.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("failed to connect to redis: %v", err)
	}

	// 连接 MongoDB
	// Path to the AWS CA file
	caFilePath := m.config.MongoDB.TLSFile
	username := m.config.MongoDB.Username
	password := m.config.MongoDB.Password
	clusterEndpoint := m.config.MongoDB.URL

	connectionString := ""
	if len(username) > 0 && len(password) > 0 {
		connectionString = fmt.Sprintf("mongodb://%s:%s@%s", username, password, clusterEndpoint)
	} else {
		connectionString = fmt.Sprintf("mongodb://%s", clusterEndpoint)
	}
	clientOptions := options.Client().ApplyURI(connectionString)
	if caFilePath != "" {
		tlsConfig, err := getCustomTLSConfig(caFilePath)
		if err != nil {
			log.Fatalf("Failed getting TLS configuration: %v", err)
		}
		clientOptions = clientOptions.SetTLSConfig(tlsConfig)
	}
	m.mongoClient, err = mongo.Connect(context.TODO(), clientOptions)
	if err != nil {
		return fmt.Errorf("failed to connect to mongodb: %v", err)
	}

	// 测试 MongoDB 连接
	if err := m.mongoClient.Ping(context.TODO(), nil); err != nil {
		return fmt.Errorf("failed to ping mongodb: %v", err)
	}

	log.Println("✅ Connected to Redis and MongoDB successfully")
	return nil
}

// Close 关闭连接
func (m *Migration) Close() error {
	if m.redisClient != nil {
		m.redisClient.Close()
	}
	if m.mongoClient != nil {
		return m.mongoClient.Disconnect(context.TODO())
	}
	return nil
}

// MigrateExpiredUsers 迁移过期用户 - 使用 goroutine 提升性能
func (m *Migration) MigrateExpiredUsers() error {
	startTime := time.Now()
	ctx := context.Background()

	// 获取所有访问时间数据
	accessData, err := m.redisClient.HGetAll(ctx, "access").Result()
	if err != nil {
		return fmt.Errorf("failed to get access data from redis: %v", err)
	}

	if len(accessData) == 0 {
		log.Println("No access data found")
		return nil
	}

	now := time.Now()
	expireThreshold := now.Add(-time.Duration(m.config.Migration.ExpireHours) * time.Hour)

	// 使用 channel 进行并发处理
	taskChan := make(chan UserTask, len(accessData))
	resultChan := make(chan UserData, len(accessData))
	errorChan := make(chan error, len(accessData))

	var wg sync.WaitGroup
	var expiredUsers []string
	var userDataList []UserData
	var errors []error

	// 启动多个 worker goroutine
	workerCount := cap(m.workerPool)
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go m.worker(ctx, taskChan, resultChan, errorChan, &wg)
	}

	// 启动结果收集 goroutine
	go func() {
		wg.Wait()
		close(resultChan)
		close(errorChan)
	}()

	// 检查每个用户的访问时间并发送任务
	go func() {
		defer close(taskChan)
		processedCount := 0
		maxUsers := m.config.Migration.MaxUsersPerRun

		for userID, accessTimeStr := range accessData {
			// 检查是否达到最大处理数量限制
			if maxUsers > 0 && processedCount >= maxUsers {
				log.Printf("Reached max users per run limit: %d", maxUsers)
				break
			}

			accessTimeUnix, err := strconv.ParseInt(accessTimeStr, 10, 64)
			if err != nil {
				log.Printf("Invalid access time for user %s: %s", userID, accessTimeStr)
				continue
			}

			accessTime := time.Unix(accessTimeUnix/1000, (accessTimeUnix%1000)*1000000)

			if accessTime.Before(expireThreshold) {
				expiredUsers = append(expiredUsers, userID)
				taskChan <- UserTask{
					UserID:        userID,
					AccessTime:    accessTime,
					AccessTimeStr: accessTimeStr,
				}
				processedCount++
			}
		}

		if maxUsers > 0 && processedCount >= maxUsers {
			log.Printf("Limited migration to %d users per run", maxUsers)
		}
	}()

	// 收集结果
	for {
		select {
		case userData, ok := <-resultChan:
			if !ok {
				goto processErrors
			}
			userDataList = append(userDataList, userData)
		case err, ok := <-errorChan:
			if !ok {
				goto processErrors
			}
			errors = append(errors, err)
		}
	}

processErrors:
	// 处理错误
	for _, err := range errors {
		log.Printf("Worker error: %v", err)
	}

	if len(userDataList) == 0 {
		log.Println("No expired users found")
		return nil
	}

	// 并发保存到 MongoDB 和删除 Redis 数据
	var saveErr, removeErr error
	var wg2 sync.WaitGroup

	wg2.Add(2)

	// 并发保存到 MongoDB
	go func() {
		defer wg2.Done()
		if err := m.saveToMongoDB(ctx, userDataList); err != nil {
			saveErr = fmt.Errorf("failed to save to mongodb: %v", err)
		}
	}()

	// 并发从 Redis 删除数据
	go func() {
		defer wg2.Done()
		if err := m.removeFromRedis(ctx, expiredUsers); err != nil {
			removeErr = fmt.Errorf("failed to remove from redis: %v", err)
		}
	}()

	wg2.Wait()

	// 更新统计信息
	m.updateStats(len(userDataList), len(errors), time.Since(startTime))

	if saveErr != nil {
		return saveErr
	}
	if removeErr != nil {
		return removeErr
	}

	log.Printf("✅ Migrated %d expired users in %v (Workers: %d)",
		len(userDataList), time.Since(startTime), workerCount)
	return nil
}

// worker 工作协程
func (m *Migration) worker(ctx context.Context, taskChan <-chan UserTask, resultChan chan<- UserData, errorChan chan<- error, wg *sync.WaitGroup) {
	defer wg.Done()

	for task := range taskChan {
		// 获取 worker 令牌
		m.workerPool <- struct{}{}

		// 处理任务
		userData, err := m.getUserData(ctx, task.UserID, task.AccessTime)

		// 释放 worker 令牌
		<-m.workerPool

		if err != nil {
			errorChan <- fmt.Errorf("failed to get user data for %s: %v", task.UserID, err)
		} else {
			resultChan <- userData
		}
	}
}

// updateStats 更新统计信息
func (m *Migration) updateStats(migratedCount, errorCount int, duration time.Duration) {
	m.stats.mu.Lock()
	defer m.stats.mu.Unlock()

	m.stats.TotalMigrated += int64(migratedCount)
	m.stats.TotalErrors += int64(errorCount)
	m.stats.LastRunTime = time.Now()
}

// GetStats 获取统计信息
func (m *Migration) GetStats() MigrationStats {
	m.stats.mu.RLock()
	defer m.stats.mu.RUnlock()

	return MigrationStats{
		TotalProcessed: m.stats.TotalProcessed,
		TotalMigrated:  m.stats.TotalMigrated,
		TotalErrors:    m.stats.TotalErrors,
		LastRunTime:    m.stats.LastRunTime,
	}
}

// getUserData 获取用户数据
func (m *Migration) getUserData(ctx context.Context, userID string, accessTime time.Time) (UserData, error) {
	var userData UserData
	userData.UserID = userID
	userData.AccessTime = accessTime
	userData.MigratedAt = time.Now()

	// // 获取用户资料
	// profileStr, err := m.redisClient.Get(ctx, fmt.Sprintf("profile:%s", userID)).Result()
	// if err == nil {
	// 	var profile Profile
	// 	if err := json.Unmarshal([]byte(profileStr), &profile); err == nil {
	// 		userData.Profile = profile
	// 	}
	// }

	// 获取用户缓存字符串
	cacheStr, err := m.redisClient.Get(ctx, fmt.Sprintf("user:%s", userID)).Result()
	if err == nil {
		userData.CacheString = cacheStr
	}

	return userData, nil
}

// saveToMongoDB 保存到 MongoDB - 使用 upsert 方式处理数据可能已存在的情况
func (m *Migration) saveToMongoDB(ctx context.Context, userDataList []UserData) error {
	collection := m.mongoClient.Database(m.config.MongoDB.Database).Collection("user")

	batchSize := m.config.Migration.BatchSize
	if batchSize <= 0 {
		batchSize = 1000 // 默认批量大小
	}

	// 分批处理
	for i := 0; i < len(userDataList); i += batchSize {
		end := i + batchSize
		if end > len(userDataList) {
			end = len(userDataList)
		}

		batch := userDataList[i:end]

		// 使用 BulkWrite 进行批量 upsert 操作
		var operations []mongo.WriteModel

		for _, userData := range batch {
			// 创建更新文档，不包含 _id 字段
			updateDoc := bson.M{
				"$set": bson.M{
					"userId":     userData.UserID,
					"accessTime": userData.AccessTime,
					"migratedAt": userData.MigratedAt,
				},
			}

			// 尝试将 CacheString 解析为 JSON 对象并直接展开到文档中
			if userData.CacheString != "" {
				var cacheData map[string]interface{}
				if err := json.Unmarshal([]byte(userData.CacheString), &cacheData); err == nil {
					// 解析成功，将 JSON 对象的字段直接添加到更新文档中
					setDoc := updateDoc["$set"].(bson.M)
					for key, value := range cacheData {
						// 确保不设置 _id 字段
						if key != "_id" {
							// 递归处理数字类型，包括嵌套对象
							setDoc[key] = m.convertNumbers(value)
						}
					}
					log.Printf("migrate user %s:%s", userData.UserID, userData.CacheString)
				} else {
					// 解析失败，存储为原始字符串
					log.Printf("Warning: Failed to parse cache string for user %s: %v", userData.UserID, err)
					log.Printf("migrate failed user %s:%s", userData.UserID, userData.CacheString)
					setDoc := updateDoc["$set"].(bson.M)
					setDoc["cacheString"] = userData.CacheString
				}
			}

			// 创建 upsert 操作，使用 userId 作为唯一标识
			operation := mongo.NewUpdateOneModel().
				SetFilter(bson.M{"userId": userData.UserID}).
				SetUpdate(updateDoc).
				SetUpsert(true)

			operations = append(operations, operation)
		}

		// 执行批量 upsert 操作
		_, err := collection.BulkWrite(ctx, operations)
		if err != nil {
			return fmt.Errorf("failed to upsert batch %d-%d: %v", i, end, err)
		}
	}

	log.Printf("Upserted %d users to MongoDB in batches of %d", len(userDataList), batchSize)
	return nil
}

// convertNumbers 递归转换数字类型，将整数从 float64 转换为 int64
func (m *Migration) convertNumbers(value interface{}) interface{} {
	switch v := value.(type) {
	case float64:
		// 检查是否为整数
		if v == float64(int64(v)) {
			return int64(v)
		}
		return v
	case map[string]interface{}:
		// 递归处理嵌套对象
		result := make(map[string]interface{})
		for key, val := range v {
			result[key] = m.convertNumbers(val)
		}
		return result
	case []interface{}:
		// 递归处理数组
		result := make([]interface{}, len(v))
		for i, val := range v {
			result[i] = m.convertNumbers(val)
		}
		return result
	default:
		return value
	}
}

// removeFromRedis 从 Redis 删除数据 - 使用批量操作提升性能
func (m *Migration) removeFromRedis(ctx context.Context, userIDs []string) error {
	batchSize := m.config.Migration.BatchSize
	if batchSize <= 0 {
		batchSize = 1000 // 默认批量大小
	}

	// 分批处理
	for i := 0; i < len(userIDs); i += batchSize {
		end := i + batchSize
		if end > len(userIDs) {
			end = len(userIDs)
		}

		batch := userIDs[i:end]
		pipe := m.redisClient.Pipeline()

		for _, userID := range batch {
			// 删除访问时间
			pipe.HDel(ctx, "access", userID)
			// 删除用户资料
			// pipe.Del(ctx, fmt.Sprintf("profile:%s", userID))
			// 删除用户缓存
			pipe.Del(ctx, fmt.Sprintf("user:%s", userID))
		}

		_, err := pipe.Exec(ctx)
		if err != nil {
			return fmt.Errorf("failed to remove batch %d-%d: %v", i, end, err)
		}
	}

	log.Printf("Removed %d users from Redis in batches of %d", len(userIDs), batchSize)
	return nil
}

// loadConfig 加载配置文件
func loadConfig(filename string) (Config, error) {
	var config Config

	file, err := os.Open(filename)
	if err != nil {
		return config, err
	}
	defer file.Close()

	bytes, err := ioutil.ReadAll(file)
	if err != nil {
		return config, err
	}

	err = json.Unmarshal(bytes, &config)
	return config, err
}
