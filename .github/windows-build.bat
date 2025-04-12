@echo off
setlocal enabledelayedexpansion

REM 该脚本用于Windows环境下直接进行构建，绕过pkg-config
echo ======= Windows直接构建脚本 =======

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
set CGO_LDFLAGS=-L%MINGW_PATH%\lib -lopus -lportaudio -lwinmm

REM 显示环境信息
echo ======= 环境信息 =======
echo MINGW_PATH: %MINGW_PATH%
echo CGO_CFLAGS: %CGO_CFLAGS%
echo CGO_LDFLAGS: %CGO_LDFLAGS%

REM 检查库文件是否存在
echo ======= 检查库文件 =======
if exist "%MINGW_PATH%\lib\libopus.a" (
    echo libopus.a 存在
) else (
    echo libopus.a 不存在！
)

if exist "%MINGW_PATH%\lib\libportaudio.a" (
    echo libportaudio.a 存在
) else (
    echo libportaudio.a 不存在！
)

REM 执行构建
echo ======= 开始构建 =======
go env
go build -v -x -o %OUTPUT_NAME% -ldflags "-X main.Version=%VERSION%" ./cmd/client

REM 检查构建结果
if %ERRORLEVEL% NEQ 0 (
    echo ======= 构建失败！=======
    exit /b %ERRORLEVEL%
) else (
    echo ======= 构建成功！=======
)

endlocal 