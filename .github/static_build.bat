@echo off
setlocal enabledelayedexpansion

REM 该脚本使用静态链接方式构建Windows版本的程序
echo ======= Windows静态链接构建脚本 =======

REM 设置基本路径
set MSYS2_PATH=C:\msys64
set MINGW_PATH=%MSYS2_PATH%\mingw64
set OUTPUT_NAME=%1

REM 确认版本信息
set VERSION=%2
if "%VERSION%"=="" set VERSION=dev
echo 构建版本: %VERSION%

REM 设置环境变量
set CGO_ENABLED=1
set CGO_CFLAGS=-I%MINGW_PATH%\include -I%MINGW_PATH%\include\opus
set CGO_LDFLAGS=-L%MINGW_PATH%\lib -lopus -lportaudio -lwinmm -static
set PATH=%MINGW_PATH%\bin;%PATH%

REM 创建临时目录
mkdir tmp_build 2>nul
cd tmp_build

REM 编译C包装器
echo ======= 编译C包装器 =======
copy ..\github\windows_opus_wrapper.c . >nul
gcc -c windows_opus_wrapper.c -o windows_opus_wrapper.o -I%MINGW_PATH%\include -I%MINGW_PATH%\include\opus
if %ERRORLEVEL% NEQ 0 (
    echo C包装器编译失败
    exit /b 1
)

REM 创建临时构建文件
echo ======= 创建构建文件 =======
echo // 临时main包装 > main_wrapper.go
echo package main >> main_wrapper.go
echo. >> main_wrapper.go
echo // #cgo CFLAGS: -I%MINGW_PATH:\=/%/include -I%MINGW_PATH:\=/%/include/opus >> main_wrapper.go
echo // #cgo LDFLAGS: -L%MINGW_PATH:\=/%/lib -lopus -lportaudio -lwinmm -static >> main_wrapper.go
echo // #cgo windows LDFLAGS: -lwinmm >> main_wrapper.go
echo // void init_audio() {} >> main_wrapper.go
echo import "C" >> main_wrapper.go
echo. >> main_wrapper.go
echo func init() { >> main_wrapper.go
echo     // 确保C初始化函数被调用 >> main_wrapper.go
echo     C.init_audio() >> main_wrapper.go
echo } >> main_wrapper.go

REM 创建临时go.mod
echo module build_temp > go.mod
echo. >> go.mod
echo replace github.com/JustaCai/xiaozhi-go => .. >> go.mod
echo. >> go.mod
echo require github.com/JustaCai/xiaozhi-go v0.0.0 >> go.mod

REM 从主项目复制源文件
mkdir -p cmd\client
copy ..\..\cmd\client\*.go cmd\client\ >nul

REM 执行构建
echo ======= 开始静态构建 =======
go build -v -o ..\%OUTPUT_NAME% -ldflags "-X main.Version=%VERSION%" github.com/JustaCai/xiaozhi-go/cmd/client

if %ERRORLEVEL% EQU 0 (
    echo ======= 构建成功！=======
    cd ..
    exit /b 0
) else (
    echo ======= 构建失败！=======
    cd ..
    exit /b 1
)

endlocal 