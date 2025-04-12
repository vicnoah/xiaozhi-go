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
set PATH=%MINGW_PATH%\bin;%PATH%

REM 显示环境信息
echo ======= 环境信息 =======
echo MINGW_PATH: %MINGW_PATH%
echo PATH: %PATH%
echo CGO_CFLAGS: %CGO_CFLAGS%
echo CGO_LDFLAGS: %CGO_LDFLAGS%

REM 检查库文件是否存在
echo ======= 检查库文件 =======
if exist "%MINGW_PATH%\lib\libopus.a" (
    echo libopus.a 存在
) else (
    echo libopus.a 不存在！
    echo 寻找可能的opus库文件:
    dir "%MINGW_PATH%\lib\*opus*.a"
)

if exist "%MINGW_PATH%\lib\libportaudio.a" (
    echo libportaudio.a 存在
) else (
    echo libportaudio.a 不存在！
    echo 寻找可能的portaudio库文件:
    dir "%MINGW_PATH%\lib\*portaudio*.a"
)

REM 检查头文件
echo ======= 检查头文件 =======
if exist "%MINGW_PATH%\include\opus\opus.h" (
    echo opus.h 存在
) else (
    echo opus.h 不存在！
    echo 寻找可能的opus头文件位置:
    dir "%MINGW_PATH%\include\opus\*.*" 2>nul || echo "opus目录不存在!"
)

if exist "%MINGW_PATH%\include\portaudio.h" (
    echo portaudio.h 存在
) else (
    echo portaudio.h 不存在！
    echo 寻找可能的portaudio头文件:
    dir "%MINGW_PATH%\include\*.h" 
)

REM 创建简单C程序测试编译 - 测试opus
echo ======= 测试Opus编译 =======
echo #include ^<stdio.h^> > opus_test.c
echo #include ^<opus/opus.h^> >> opus_test.c
echo int main() { printf("Opus Test\n"); return 0; } >> opus_test.c

gcc opus_test.c -o opus_test.exe -I%MINGW_PATH%\include -L%MINGW_PATH%\lib -lopus
if %ERRORLEVEL% EQU 0 (
    echo Opus库编译测试成功
) else (
    echo Opus库编译测试失败
)

REM 执行构建
echo ======= 开始构建 =======
go version
go env

REM 尝试使用标准方法构建
echo 方法1：标准构建
go build -v -o %OUTPUT_NAME% -ldflags "-X main.Version=%VERSION%" ./cmd/client
if %ERRORLEVEL% EQU 0 goto :buildsuccess

REM 尝试使用替代方法
echo 方法2：使用特殊构建参数
go build -v -x -o %OUTPUT_NAME% -ldflags "-X main.Version=%VERSION%" -tags=nogui ./cmd/client
if %ERRORLEVEL% EQU 0 goto :buildsuccess

echo 方法3：使用C包装
go build -v -o %OUTPUT_NAME% -ldflags "-X main.Version=%VERSION%" -tags=static ./cmd/client

:buildsuccess
REM 检查构建结果
if exist %OUTPUT_NAME% (
    echo ======= 构建成功！=======
    exit /b 0
) else (
    echo ======= 构建失败！=======
    exit /b 1
)

endlocal 