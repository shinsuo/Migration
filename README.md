# Migration Service

这是一个 Go 程序，用于定期将 Redis 中过期的用户数据迁移到 MongoDB。

## 功能特性

- 🔄 每 10 分钟检查一次 Redis 中的用户访问时间
- ⏰ 自动识别超过 6 小时未访问的用户
- 💾 将过期用户数据备份到 MongoDB
- 🗑️ 从 Redis 中删除过期用户数据
- ⚙️ 支持配置文件自定义参数
- 🚀 **高性能并发处理**：使用 goroutine 和 worker pool 模式
- 📊 **批量操作**：MongoDB 和 Redis 批量处理提升性能
- 📈 **实时统计**：监控迁移进度和性能指标
- 🔧 **生产就绪**：完善的错误处理和资源管理

## 文件结构

```
Migration/
├── main.go              # 主程序入口
├── types.go             # 共享类型定义和方法
├── benchmark.go         # 性能测试工具
├── go.mod              # Go 模块文件
├── config.json         # 配置文件
├── start.bat           # Windows 启动脚本
├── start.sh           # Linux/Mac 启动脚本
├── benchmark.bat       # Windows 性能测试脚本
├── benchmark.sh       # Linux/Mac 性能测试脚本
├── Dockerfile         # Docker 镜像构建文件
├── docker-compose.yml # Docker Compose 配置
└── README.md          # 使用说明
```

## 数据结构

### Redis 数据结构

- `access` (Hash): 存储用户访问时间
    - Key: `userId`
    - Value: `timestamp` (毫秒时间戳)
- `profile:{userId}` (String): 用户资料 JSON
- `user:{userId}` (String): 用户缓存数据

### MongoDB 数据结构

- Collection: `migrated_users`
- Document: 包含完整的用户数据、访问时间和迁移时间

## 安装和运行

### 1. 安装 Go

确保已安装 Go 1.21 或更高版本。

### 2. 配置

编辑 `config.json` 文件：

```json
{
    "redis": {
        "url": "redis://localhost:6379/0",
        "host": "localhost",
        "port": 6379,
        "db": 0
    },
    "mongodb": {
        "url": "mongodb://localhost:27017",
        "host": "localhost",
        "port": 27017,
        "database": "app"
    },
    "migration": {
        "checkIntervalMinutes": 10,
        "expireHours": 6,
        "batchSize": 100
    }
}
```

### 3. 运行

#### Linux/Mac:

```bash
# 运行主程序
chmod +x start.sh
./start.sh

# 运行性能测试
chmod +x benchmark.sh
./benchmark.sh
```

#### Windows:

```cmd
# 运行主程序
start.bat

# 运行性能测试
benchmark.bat
```

#### 手动运行:

```bash
# 运行主程序
go run main.go types.go

# 运行性能测试
go run benchmark.go types.go

# 或者构建后运行
go build -o migration main.go types.go
./migration
```

## 配置说明

- `checkIntervalMinutes`: 检查间隔（分钟）
- `expireHours`: 过期时间（小时）
- `batchSize`: 批处理大小（MongoDB 和 Redis 批量操作）
- `workerCount`: Worker 协程数量（0 表示自动设置为 CPU 核心数 × 2）
- `maxConcurrency`: 最大并发数限制
- `redis.url`: Redis 连接 URL
- `mongodb.url`: MongoDB 连接 URL
- `mongodb.database`: MongoDB 数据库名

### 性能优化配置建议

**生产环境推荐配置：**

```json
{
    "migration": {
        "checkIntervalMinutes": 5,
        "expireHours": 6,
        "batchSize": 2000,
        "workerCount": 16,
        "maxConcurrency": 200
    }
}
```

**高负载环境配置：**

```json
{
    "migration": {
        "checkIntervalMinutes": 2,
        "expireHours": 4,
        "batchSize": 5000,
        "workerCount": 32,
        "maxConcurrency": 500
    }
}
```

## 日志输出

程序会输出详细的日志信息：

- ✅ 成功连接数据库
- 🔄 开始迁移任务
- 📊 迁移的用户数量和耗时
- 📈 实时统计信息
- ⚡ Worker 协程数量
- ❌ 错误信息

### 性能监控

程序提供实时性能统计：

- 总迁移用户数
- 总错误数
- 最后运行时间
- 处理耗时
- Worker 协程使用情况

## 注意事项

1. 确保 Redis 和 MongoDB 服务正在运行
2. 确保有足够的权限访问数据库
3. 建议在生产环境中使用进程管理器（如 systemd, PM2）
4. 定期备份 MongoDB 数据

## 错误处理

程序包含完善的错误处理机制：

- 数据库连接失败
- 数据解析错误
- 网络超时
- 批量操作失败

## 扩展功能

可以根据需要扩展以下功能：

- 支持多种数据源
- 添加数据验证
- 实现数据压缩
- 添加监控指标
- 支持分布式部署
