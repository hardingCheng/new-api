#!/bin/bash

# 开发环境构建脚本

# 颜色定义
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 进度显示函数
show_progress() {
    local current=$1
    local total=$2
    local message=$3
    local percent=$((current * 100 / total))
    echo -e "${BLUE}[${current}/${total}]${NC} ${message} ${GREEN}(${percent}%)${NC}"
}

case "$1" in
  web)
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "  🔨 构建前端"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    
    show_progress 1 3 "进入 web 目录..."
    cd web
    
    show_progress 2 3 "安装依赖..."
    bun install
    
    show_progress 3 3 "构建前端..."
    bun run build
    
    echo ""
    echo -e "${GREEN}✅ 前端构建完成！${NC}"
    echo -e "${YELLOW}💡 刷新浏览器即可看到更新（无需重启容器）${NC}"
    echo ""
    ;;
    
  go)
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "  🔨 构建后端"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    
    show_progress 1 2 "读取版本号..."
    VERSION=$(cat VERSION)
    
    show_progress 2 2 "编译 Go 程序..."
    go build -ldflags "-s -w -X 'github.com/QuantumNous/new-api/common.Version=${VERSION}'" -o new-api
    
    echo ""
    echo -e "${GREEN}✅ 后端构建完成！${NC}"
    echo -e "${YELLOW}💡 执行以下命令重启容器：${NC}"
    echo -e "   ${BLUE}./dev-build.sh restart${NC}"
    echo ""
    ;;
    
  all)
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "  🔨 构建前端和后端"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    
    # 前端构建
    echo -e "${BLUE}▶ 第 1 部分：前端${NC}"
    show_progress 1 5 "进入 web 目录..."
    cd web
    
    show_progress 2 5 "安装前端依赖..."
    bun install
    
    show_progress 3 5 "构建前端..."
    bun run build
    cd ..
    
    echo ""
    echo -e "${BLUE}▶ 第 2 部分：后端${NC}"
    show_progress 4 5 "读取版本号..."
    VERSION=$(cat VERSION)
    
    show_progress 5 5 "编译 Go 程序..."
    go build -ldflags "-s -w -X 'github.com/QuantumNos/new-api/common.Version=${VERSION}'" -o new-api
    
    echo ""
    echo -e "${GREEN}✅ 全部构建完成！${NC}"
    echo -e "${YELLOW}💡 现在可以启动容器：${NC}"
    echo -e "   ${BLUE}docker-compose -f docker-compose.dev.yml up -d${NC}"
    echo ""
    ;;
    
  restart)
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "  🔄 重启容器"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    
    show_progress 1 2 "重启容器..."
    docker-compose -f docker-compose.dev.yml restart
    
    show_progress 2 2 "等待服务启动..."
    sleep 2
    
    echo ""
    echo -e "${GREEN}✅ 容器已重启${NC}"
    echo -e "${YELLOW}💡 查看日志：${NC}"
    echo -e "   ${BLUE}docker-compose -f docker-compose.dev.yml logs -f${NC}"
    echo ""
    ;;
    
  *)
    echo ""
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "  📖 开发构建工具"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    echo "用法: ./dev-build.sh [命令]"
    echo ""
    echo "可用命令："
    echo "  ${GREEN}web${NC}     - 只构建前端（最常用，3-5秒）"
    echo "  ${GREEN}go${NC}      - 只构建后端"
    echo "  ${GREEN}all${NC}     - 构建前端和后端"
    echo "  ${GREEN}restart${NC} - 重启容器"
    echo ""
    echo "示例："
    echo "  ${BLUE}./dev-build.sh web${NC}      # 修改前端后执行"
    echo "  ${BLUE}./dev-build.sh go${NC}       # 修改后端后执行"
    echo "  ${BLUE}./dev-build.sh restart${NC}  # 重启容器"
    echo ""
    exit 1
    ;;
esac
