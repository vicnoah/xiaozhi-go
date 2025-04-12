#!/bin/bash

# 定义颜色
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # 无颜色

# 定义环境变量
APP_NAME="xiaozhi-client"
VERSION="1.0.0"
BOARD_TYPE="generic"

# 打印带颜色的信息
print_info() {
    echo -e "${GREEN}[信息] $1${NC}"
}

print_warn() {
    echo -e "${YELLOW}[警告] $1${NC}"
}

print_error() {
    echo -e "${RED}[错误] $1${NC}"
}

# 显示帮助信息
show_help() {
    echo "用法: $0 [选项]"
    echo ""
    echo "选项:"
    echo "  -h, --help        显示帮助信息"
    echo "  -b, --build       只编译程序"
    echo "  -r, --run         编译并运行程序"
    echo "  -c, --clean       清理编译产物"
    echo "  -a, --activate    只执行设备激活"
    echo "  -v, --version     指定版本号 (默认: ${VERSION})"
    echo "  -t, --board-type  指定设备板型号 (默认: ${BOARD_TYPE})"
    echo "  --server          指定WebSocket服务器地址"
    echo "  --token           指定API访问令牌"
    echo ""
    echo "例子:"
    echo "  $0 --build                  # 只编译程序"
    echo "  $0 --run                    # 编译并运行程序"
    echo "  $0 --clean                  # 清理编译产物"
    echo "  $0 --run --server wss://example.com  # 使用自定义服务器地址"
    echo "  $0 --activate               # 执行设备激活"
}

# 检查依赖
check_dependencies() {
    print_info "检查依赖..."
    
    # 检查Go
    if ! command -v go &> /dev/null; then
        print_error "未安装Go，请先安装Go: https://golang.org/doc/install"
        exit 1
    fi
    
    print_info "Go版本: $(go version)"
    
    # 检查需要的Go包
    go_deps=(
        "github.com/gorilla/websocket"
        "github.com/sirupsen/logrus"
        "github.com/google/uuid"
    )
    
    for dep in "${go_deps[@]}"; do
        if ! go list -m $dep &> /dev/null; then
            print_info "安装依赖: $dep"
            go get $dep
        fi
    done
    
    print_info "所有依赖已满足"
}

# 清理编译产物
clean() {
    print_info "清理编译产物..."
    if [ -f "$APP_NAME" ]; then
        rm "$APP_NAME"
        print_info "已删除 $APP_NAME"
    else
        print_warn "未找到 $APP_NAME，无需清理"
    fi
}

# 编译程序
build() {
    print_info "编译程序..."
    
    # 显示当前版本
    print_info "使用版本号: $VERSION"
    print_info "设备板型号: $BOARD_TYPE"
    
    # 执行编译
    go build -o "$APP_NAME" -ldflags "-X main.Version=$VERSION" ./cmd/client
    
    if [ $? -ne 0 ]; then
        print_error "编译失败!"
        exit 1
    fi
    
    print_info "编译成功: $APP_NAME"
}

# 准备运行参数
prepare_run_args() {
    RUN_ARGS=""
    
    # 添加版本参数
    RUN_ARGS="$RUN_ARGS -version $VERSION"
    
    # 添加板型号参数
    RUN_ARGS="$RUN_ARGS -board $BOARD_TYPE"
    
    # 添加WebSocket服务器地址
    if [ ! -z "$SERVER_URL" ]; then
        RUN_ARGS="$RUN_ARGS -server $SERVER_URL"
    fi
    
    # 添加访问令牌
    if [ ! -z "$TOKEN" ]; then
        RUN_ARGS="$RUN_ARGS -token $TOKEN"
    fi
    
    # 添加激活标志
    if [ "$ACTIVATE_ONLY" = true ]; then
        RUN_ARGS="$RUN_ARGS -activate-only"
    fi
}

# 运行程序
run() {
    print_info "运行程序..."
    
    # 准备运行参数
    prepare_run_args
    
    print_info "启动命令: ./$APP_NAME $RUN_ARGS"
    
    # 运行程序
    ./$APP_NAME $RUN_ARGS
}

# 主函数
main() {
    # 默认行为
    DO_BUILD=false
    DO_RUN=false
    DO_CLEAN=false
    ACTIVATE_ONLY=false
    SERVER_URL=""
    TOKEN=""
    
    # 解析命令行参数
    while [[ $# -gt 0 ]]; do
        case $1 in
            -h|--help)
                show_help
                exit 0
                ;;
            -b|--build)
                DO_BUILD=true
                shift
                ;;
            -r|--run)
                DO_BUILD=true
                DO_RUN=true
                shift
                ;;
            -c|--clean)
                DO_CLEAN=true
                shift
                ;;
            -a|--activate)
                ACTIVATE_ONLY=true
                DO_BUILD=true
                DO_RUN=true
                shift
                ;;
            -v|--version)
                VERSION="$2"
                shift
                shift
                ;;
            -t|--board-type)
                BOARD_TYPE="$2"
                shift
                shift
                ;;
            --server)
                SERVER_URL="$2"
                shift
                shift
                ;;
            --token)
                TOKEN="$2"
                shift
                shift
                ;;
            *)
                print_error "未知选项: $1"
                show_help
                exit 1
                ;;
        esac
    done
    
    # 如果没有指定操作，则显示帮助
    if [ "$DO_BUILD" = false ] && [ "$DO_RUN" = false ] && [ "$DO_CLEAN" = false ]; then
        show_help
        exit 0
    fi
    
    # 检查依赖
    check_dependencies
    
    # 清理
    if [ "$DO_CLEAN" = true ]; then
        clean
    fi
    
    # 编译
    if [ "$DO_BUILD" = true ]; then
        build
    fi
    
    # 运行
    if [ "$DO_RUN" = true ]; then
        run
    fi
    
    print_info "操作完成!"
}

# 执行主函数
main "$@" 