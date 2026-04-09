@echo off
chcp 65001 >nul
echo ========================================
echo   go2rtc Windows 服务卸载脚本
echo ========================================
echo.

:: 检查管理员权限
net session >nul 2>&1
if %errorLevel% neq 0 (
    echo [错误] 请以管理员身份运行此脚本
    echo.
    pause
    exit /b 1
)

echo [√] 已获取管理员权限
echo.

:: 获取脚本所在目录
set SCRIPT_DIR=%~dp0
set NSSM_EXE=%SCRIPT_DIR%nssm\win64\nssm.exe

:: 检查 NSSM
if not exist "%NSSM_EXE%" (
    echo [错误] 找不到 NSSM，请确保安装脚本运行过
    pause
    exit /b 1
)

:: 确认卸载
echo 即将卸载 go2rtc 服务
echo.
set /p CONFIRM="确认要删除服务吗？(Y/N): "
if /i not "%CONFIRM%"=="Y" (
    echo.
    echo [取消] 卸载已取消
    pause
    exit /b 0
)

echo.
echo [提示] 正在停止服务...
"%NSSM_EXE%" stop go2rtc

echo [提示] 正在删除服务...
"%NSSM_EXE%" remove go2rtc confirm

if %errorLevel% neq 0 (
    echo [错误] 服务删除失败
    pause
    exit /b 1
)

echo.
echo ========================================
echo   go2rtc 服务已卸载
echo ========================================
echo.
echo 以下文件未被删除，可手动清理:
echo   - %SCRIPT_DIR%go2rtc.exe
echo   - %SCRIPT_DIR%go2rtc.yaml
echo   - %SCRIPT_DIR%nssm\
echo   - %SCRIPT_DIR%*.log
echo.

pause
