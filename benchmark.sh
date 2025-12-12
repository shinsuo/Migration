#!/bin/bash

# Migration 性能测试脚本

echo "🚀 Starting Migration Performance Test..."

# 检查 Go 是否安装
if ! command -v go &> /dev/null; then
    echo "❌ Go is not installed. Please install Go first."
    exit 1
fi

# 进入 Migration 目录
cd "$(dirname "$0")"

# 检查配置文件
if [ ! -f "config.json" ]; then
    echo "❌ config.json not found. Please create it first."
    exit 1
fi

# 下载依赖
echo "📦 Downloading dependencies..."
go mod tidy

# 运行性能测试
echo "🔨 Running Performance Benchmark..."
go run benchmark.go types.go

if [ $? -eq 0 ]; then
    echo "✅ Performance test completed!"
else
    echo "❌ Performance test failed!"
    exit 1
fi
