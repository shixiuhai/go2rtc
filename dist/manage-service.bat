@echo off
chcp 65001 >nul

:: 获取脚本所在目录
set SCRIPT_DIR=%~dp0
set NSSM_EXE=%SCRIPT_DIR%nssm\win64\nssm.exe

:: 显示菜单
echo ========================================
echo   go2rtc 服务管理
echo ========================================
echo.

sc query go2rtc | findstr "RUNNING" >nul
if %errorLevel% equ 0 (
    echo 服务状态：[运行中]
) else (
    echo 服务状态：[已停止]
)
echo.

echo 请选择操作:
echo   1. 启动服务
echo   2. 停止服务
echo   3. 重启服务
echo   4. 查看日志
echo   5. 测试 API
echo   6. 退出
echo.

set /p CHOICE="请输入选项 (1-6): "

if "%CHOICE%"=="1" (
    echo [提示] 正在启动服务...
    "%NSSM_EXE%" start go2rtc
    echo 完成！
) else if "%CHOICE%"=="2" (
    echo [提示] 正在停止服务...
    "%NSSM_EXE%" stop go2rtc
    echo 完成！
) else if "%CHOICE%"=="3" (
    echo [提示] 正在重启服务...
    "%NSSM_EXE%" restart go2rtc
    echo 完成！
) else if "%CHOICE%"=="4" (
    if exist "%SCRIPT_DIR%go2rtc-service.log" (
        notepad "%SCRIPT_DIR%go2rtc-service.log"
    ) else (
        echo 日志文件不存在
    )
) else if "%CHOICE%"=="5" (
    echo [提示] 测试 API...
    curl http://localhost:1984/api/health
) else if "%CHOICE%"=="6" (
    exit /b 0
)

echo.
pause
