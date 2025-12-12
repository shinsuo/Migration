@echo off
REM Migration 启动脚本 (Windows)

echo 🚀 Starting Migration Service...

REM 检查 Go 是否安装
go version >nul 2>&1
if %errorlevel% neq 0 (
    echo ❌ Go is not installed. Please install Go first.
    pause
    exit /b 1
)

REM 检查配置文件
if not exist "config.json" (
    echo ❌ config.json not found. Please create it first.
    pause
    exit /b 1
)

REM 下载依赖
echo 📦 Downloading dependencies...
go mod tidy

REM 构建程序
echo 🔨 Building Migration...
go build -o migration.exe main.go types.go

if %errorlevel% equ 0 (
    echo ✅ Build successful!
    echo 🚀 Starting Migration service...
    migration.exe
) else (
    echo ❌ Build failed!
    pause
    exit /b 1
)
