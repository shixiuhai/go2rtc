@echo off
chcp 65001 >nul
echo ========================================
echo   go2rtc Windows 服务安装脚本
echo ========================================
echo.

:: 检查管理员权限
net session >nul 2>&1
if %errorLevel% neq 0 (
    echo [错误] 请以管理员身份运行此脚本
    echo.
    echo 右键点击脚本 -> 以管理员身份运行
    pause
    exit /b 1
)

echo [√] 已获取管理员权限
echo.

:: 获取脚本所在目录
set SCRIPT_DIR=%~dp0

:: 自动查找 go2rtc.exe（支持多个版本）
set GO2RTC_EXE=%SCRIPT_DIR%go2rtc_windows_amd64.exe
if not exist "%GO2RTC_EXE%" (
    set GO2RTC_EXE=%SCRIPT_DIR%go2rtc.exe
)
if not exist "%GO2RTC_EXE%" (
    set GO2RTC_EXE=%SCRIPT_DIR%go2rtc_windows.exe
)

set GO2RTC_CONFIG=%SCRIPT_DIR%go2rtc.yaml

:: 检查文件是否存在
if not exist "%GO2RTC_EXE%" (
    echo [错误] 找不到 go2rtc.exe
    echo 位置：%GO2RTC_EXE%
    pause
    exit /b 1
)

echo [√] 找到 go2rtc.exe
echo     %GO2RTC_EXE%
echo.

:: 创建配置文件（如果不存在）
if not exist "%GO2RTC_CONFIG%" (
    echo [提示] 创建默认配置文件...
    (
        echo api:
        echo   listen: ":1984"
        echo   origin: "*"
        echo.
        echo rtsp:
        echo   port: 8554
        echo.
        echo conversion:
        echo   max_streams: 50
    ) > "%GO2RTC_CONFIG%"
    echo [√] 配置文件已创建：%GO2RTC_CONFIG%
    echo.
) else (
    echo [√] 配置文件已存在：%GO2RTC_CONFIG%
    echo.
)

:: 下载 NSSM（如果不存在）
set NSSM_DIR=%SCRIPT_DIR%nssm
set NSSM_EXE=%NSSM_DIR%\win64\nssm.exe

if not exist "%NSSM_EXE%" (
    echo [提示] 下载 NSSM 服务管理工具...
    mkdir "%NSSM_DIR%" 2>nul
    
    :: 尝试使用 PowerShell 下载
    powershell -Command "Start-BitsTransfer -Source 'https://nssm.cc/release/nssm-2.24.zip' -Destination '%TEMP%\nssm.zip' 2>$null"
    
    if not exist "%TEMP%\nssm.zip" (
        echo [警告] 自动下载失败，请手动下载 NSSM
        echo.
        echo 下载地址：https://nssm.cc/download
        echo 下载后解压到：%NSSM_DIR%
        echo.
        pause
    ) else (
        echo [√] 下载完成，解压中...
        powershell -Command "Expand-Archive -Path '%TEMP%\nssm.zip' -DestinationPath '%NSSM_DIR%' -Force"
        del "%TEMP%\nssm.zip"
        echo [√] NSSM 已安装
        echo.
    )
) else (
    echo [√] NSSM 已存在
    echo.
)

:: 检查 NSSM
if not exist "%NSSM_EXE%" (
    echo [错误] 找不到 NSSM，请手动下载
    pause
    exit /b 1
)

:: 停止并删除旧服务（如果存在）
echo [提示] 检查旧服务...
sc query go2rtc >nul 2>&1
if %errorLevel% equ 0 (
    echo [提示] 发现旧服务，正在删除...
    "%NSSM_EXE%" stop go2rtc
    "%NSSM_EXE%" remove go2rtc confirm
    echo [√] 旧服务已删除
    echo.
)

:: 安装服务
echo [提示] 正在安装 go2rtc 服务...
"%NSSM_EXE%" install go2rtc "%GO2RTC_EXE%" -config "%GO2RTC_CONFIG%"

if %errorLevel% neq 0 (
    echo [错误] 服务安装失败
    pause
    exit /b 1
)

echo [√] 服务安装成功
echo.

:: 配置服务
echo [提示] 配置服务参数...

:: 设置启动目录
"%NSSM_EXE%" set go2rtc AppDirectory "%SCRIPT_DIR%"

:: 设置重启策略
"%NSSM_EXE%" set go2rtc AppRestartDelay 1000
"%NSSM_EXE%" set go2rtc AppThrottle 1000

:: 设置日志
"%NSSM_EXE%" set go2rtc AppStdout "%SCRIPT_DIR%go2rtc-service.log"
"%NSSM_EXE%" set go2rtc AppStderr "%SCRIPT_DIR%go2rtc-error.log"

:: 设置自动启动
"%NSSM_EXE%" set go2rtc StartService auto

echo [√] 服务配置完成
echo.

:: 启动服务
echo [提示] 正在启动服务...
"%NSSM_EXE%" start go2rtc

if %errorLevel% neq 0 (
    echo [错误] 服务启动失败
    pause
    exit /b 1
)

echo [√] 服务已启动
echo.

:: 验证服务
timeout /t 2 >nul
sc query go2rtc | findstr "RUNNING" >nul
if %errorLevel% equ 0 (
    echo ========================================
    echo   go2rtc 服务安装成功！
    echo ========================================
    echo.
    echo 服务名称：go2rtc
    echo 服务状态：运行中
    echo API 地址：http://localhost:1984
    echo 配置文件：%GO2RTC_CONFIG%
    echo 日志文件：%SCRIPT_DIR%go2rtc-service.log
    echo.
    echo 管理命令:
    echo   停止服务：%NSSM_EXE% stop go2rtc
    echo   重启服务：%NSSM_EXE% restart go2rtc
    echo   删除服务：%NSSM_EXE% remove go2rtc confirm
    echo.
    echo 测试 API:
    echo   curl http://localhost:1984/api/health
    echo.
) else (
    echo [警告] 服务可能未正常启动，请检查日志
    echo 日志位置：%SCRIPT_DIR%go2rtc-service.log
)

pause
