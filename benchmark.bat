@echo off
REM Migration 性能测试脚本 (Windows)

echo 🚀 Starting Migration Performance Test...

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

REM 运行性能测试
echo 🔨 Running Performance Benchmark...
go run benchmark.go types.go

if %errorlevel% equ 0 (
    echo ✅ Performance test completed!
) else (
    echo ❌ Performance test failed!
    pause
    exit /b 1
)

pause
