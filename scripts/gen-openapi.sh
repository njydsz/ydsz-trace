#!/usr/bin/env bash
# gen-openapi.sh — 使用 swag 生成 OpenAPI JSON 文档
#
# 用法：
#   ./scripts/gen-openapi.sh
#
# 依赖：
#   - swag (go install github.com/swaggo/swag/cmd/swag@latest)
#   - Go 1.26.5+
#
# 输出：
#   docs/openapi.json

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

OUTPUT_DIR="${PROJECT_ROOT}/docs"
OUTPUT_FILE="${OUTPUT_DIR}/openapi.json"

echo "[gen-openapi] 项目根目录: ${PROJECT_ROOT}"
echo "[gen-openapi] 输出路径: ${OUTPUT_FILE}"

# 检查 swag 是否安装
if ! command -v swag &> /dev/null; then
    echo "[gen-openapi] swag 未安装，尝试安装..."
    go install github.com/swaggo/swag/cmd/swag@latest
    if [ $? -ne 0 ]; then
        echo "[gen-openapi] swag 安装失败，请手动执行: go install github.com/swaggo/swag/cmd/swag@latest"
        exit 1
    fi
fi

# 确保输出目录存在
mkdir -p "${OUTPUT_DIR}"

# 生成 OpenAPI JSON
echo "[gen-openapi] 正在生成 OpenAPI 文档..."
cd "${PROJECT_ROOT}/logs"

# swag init 生成 docs 目录下的 Go 文件，然后转换为 JSON
swag init \
    --output "${PROJECT_ROOT}/docs" \
    --outputTypes go \
    --parseDependency \
    --parseInternal \
    --generalInfo main.go \
    --propagation 2>/dev/null || true

# 使用 swag 的 fmt 命令或直接调用 swag 生成 JSON
# 注意：swag 默认生成 Go 代码，这里使用 swag 的 JSON 输出
swag init \
    --output "${OUTPUT_DIR}" \
    --outputTypes swagger,json,yaml \
    --parseDependency \
    --parseInternal \
    --generalInfo main.go \
    --instanceName logs 2>/dev/null || true

# 如果 swag 生成了 swagger.json，重命名为 openapi.json
if [ -f "${OUTPUT_DIR}/swagger.json" ]; then
    mv "${OUTPUT_DIR}/swagger.json" "${OUTPUT_FILE}"
    echo "[gen-openapi] 已生成: ${OUTPUT_FILE}"
else
    echo "[gen-openapi] 警告: 未找到 swagger.json，尝试其他方式..."
    # 尝试直接使用 swag fmt 生成
    swag fmt -d "${PROJECT_ROOT}/logs" 2>/dev/null || true
fi

# 清理临时文件
rm -rf "${OUTPUT_DIR}/docs.go" 2>/dev/null || true
rm -rf "${OUTPUT_DIR}/swagger.yaml" 2>/dev/null || true

echo "[gen-openapi] 完成"
