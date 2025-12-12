#!/bin/bash

# Migration 启动脚本

echo "🚀 Starting Migration Service..."

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

# 构建程序
echo "🔨 Building Migration..."
go build -o migration main.go types.go

if [ $? -eq 0 ]; then
    echo "✅ Build successful!"
    echo "🚀 Starting Migration service..."
    ./migration
else
    echo "❌ Build failed!"
    exit 1
fi
