#!/bin/bash

# 开发工具安装脚本（适用于 Debian/Ubuntu Linux）
# 自动安装最新版本的 Go 和 Bun

set -e

echo "🚀 开始安装开发工具..."
echo ""

# 检测系统
if [ -f /etc/debian_version ]; then
    echo "✅ 检测到 Debian/Ubuntu 系统"
else
    echo "⚠️  警告：此脚本为 Debian/Ubuntu 设计，其他系统可能需要调整"
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  第 1 步：安装 Go (最新稳定版)"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

if command -v go &> /dev/null; then
    CURRENT_GO_VERSION=$(go version | awk '{print $3}')
    echo "✅ Go 已安装: $CURRENT_GO_VERSION"
    read -p "是否重新安装最新版？(y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "⏭️  跳过 Go 安装"
    else
        INSTALL_GO=true
    fi
else
    INSTALL_GO=true
fi

if [ "$INSTALL_GO" = true ]; then
    echo "🔍 获取最新 Go 版本..."
    GO_VERSION=$(curl -s https://go.dev/VERSION?m=text | head -n 1 | sed 's/go//')
    echo "� 下载 Go ${GO_VERSION}..."
    
    wget --progress=bar:force:noscroll https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz 2>&1 | \
        grep -o '[0-9]*%' | while read percent; do
            echo -ne "\r下载进度: $percent"
        done
    echo ""
    
    echo "📦 解压安装..."
    sudo rm -rf /usr/local/go
    sudo tar -C /usr/local -xzf go${GO_VERSION}.linux-amd64.tar.gz
    rm go${GO_VERSION}.linux-amd64.tar.gz
    
    # 添加到 PATH
    if ! grep -q "/usr/local/go/bin" ~/.bashrc; then
        echo "" >> ~/.bashrc
        echo '# Go environment' >> ~/.bashrc
        echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
        echo 'export PATH=$PATH:~/go/bin' >> ~/.bashrc
    fi
    
    export PATH=$PATH:/usr/local/go/bin
    echo "✅ Go ${GO_VERSION} 安装完成！"
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  第 2 步：安装 Bun (最新版)"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

if command -v bun &> /dev/null; then
    CURRENT_BUN_VERSION=$(bun --version)
    echo "✅ Bun 已安装: v$CURRENT_BUN_VERSION"
    read -p "是否重新安装最新版？(y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "⏭️  跳过 Bun 安装"
    else
        INSTALL_BUN=true
    fi
else
    INSTALL_BUN=true
fi

if [ "$INSTALL_BUN" = true ]; then
    echo "📥 下载并安装 Bun..."
    curl -fsSL https://bun.sh/install | bash
    
    # 添加到 PATH
    export BUN_INSTALL="$HOME/.bun"
    export PATH="$BUN_INSTALL/bin:$PATH"
    
    if [ -f ~/.bashrc ] && ! grep -q "BUN_INSTALL" ~/.bashrc; then
        echo "" >> ~/.bashrc
        echo '# Bun environment' >> ~/.bashrc
        echo 'export BUN_INSTALL="$HOME/.bun"' >> ~/.bashrc
        echo 'export PATH="$BUN_INSTALL/bin:$PATH"' >> ~/.bashrc
    fi
    
    echo "✅ Bun $(bun --version) 安装完成！"
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  第 3 步：安装 make"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

if command -v make &> /dev/null; then
    echo "✅ make 已安装"
else
    echo "📦 安装 make..."
    sudo apt update -qq
    sudo apt install -y make
    echo "✅ make 安装完成"
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  🎉 安装完成！"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "已安装的工具："
echo "  • Go:   $(go version 2>/dev/null || echo '需要重新加载环境变量')"
echo "  • Bun:  v$(bun --version 2>/dev/null || echo '需要重新加载环境变量')"
echo "  • make: $(make --version 2>/dev/null | head -n 1 || echo '已安装')"
echo ""
echo "⚠️  重要：请执行以下命令使环境变量生效："
echo ""
echo "    source ~/.bashrc"
echo ""
echo "然后验证安装："
echo ""
echo "    go version"
echo "    bun --version"
echo ""
