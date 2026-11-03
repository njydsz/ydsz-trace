@echo off
REM gen-openapi.bat — 使用 swag 生成 OpenAPI JSON 文档（Windows 版本）
REM
REM 用法：
REM   scripts\gen-openapi.bat
REM
REM 依赖：
REM   - swag (go install github.com/swaggo/swag/cmd/swag@latest)
REM   - Go 1.26.5+
REM
REM 输出：
REM   docs\openapi.json

setlocal enabledelayedexpansion

set "SCRIPT_DIR=%~dp0"
set "PROJECT_ROOT=%SCRIPT_DIR%.."
set "OUTPUT_DIR=%PROJECT_ROOT%\docs"
set "OUTPUT_FILE=%OUTPUT_DIR%\openapi.json"

echo [gen-openapi] 项目根目录: %PROJECT_ROOT%
echo [gen-openapi] 输出路径: %OUTPUT_FILE%

REM 检查 swag 是否安装
where swag >nul 2>nul
if %errorlevel% neq 0 (
    echo [gen-openapi] swag 未安装，尝试安装...
    go install github.com/swaggo/swag/cmd/swag@latest
    if %errorlevel% neq 0 (
        echo [gen-openapi] swag 安装失败，请手动执行: go install github.com/swaggo/swag/cmd/swag@latest
        exit /b 1
    )
)

REM 确保输出目录存在
if not exist "%OUTPUT_DIR%" mkdir "%OUTPUT_DIR%"

REM 生成 OpenAPI JSON
echo [gen-openapi] 正在生成 OpenAPI 文档...
cd /d "%PROJECT_ROOT%\logs"

REM swag init 生成文档
swag init ^
    --output "%OUTPUT_DIR%" ^
    --outputTypes swagger,json ^
    --parseDependency ^
    --parseInternal ^
    --generalInfo main.go ^
    --instanceName logs

if %errorlevel% neq 0 (
    echo [gen-openapi] swag init 失败
    exit /b 1
)

REM 如果 swag 生成了 swagger.json，重命名为 openapi.json
if exist "%OUTPUT_DIR%\swagger.json" (
    move /Y "%OUTPUT_DIR%\swagger.json" "%OUTPUT_FILE%" >nul
    echo [gen-openapi] 已生成: %OUTPUT_FILE%
) else (
    echo [gen-openapi] 警告: 未找到 swagger.json
)

REM 清理临时文件
if exist "%OUTPUT_DIR%\docs.go" del /Q "%OUTPUT_DIR%\docs.go"
if exist "%OUTPUT_DIR%\swagger.yaml" del /Q "%OUTPUT_DIR%\swagger.yaml"

echo [gen-openapi] 完成
endlocal
