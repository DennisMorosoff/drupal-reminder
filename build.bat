@echo off
REM Скрипт сборки Drupal Reminder Bot с автоматическим определением версии из git

setlocal enabledelayedexpansion

echo Building Drupal Reminder Bot...
echo.

REM Определение версии из git тегов
for /f "delims=" %%i in ('git describe --tags --always --dirty 2^>nul') do set VERSION=%%i
if "%VERSION%"=="" set VERSION=dev

REM Определение времени сборки в формате ISO 8601 (используем PowerShell для точного формата)
for /f "delims=" %%i in ('powershell -Command "Get-Date -Format 'yyyy-MM-ddTHH:mm:ssZ' -AsUTC"') do set BUILD_TIME=%%i
if "%BUILD_TIME%"=="" (
    REM Fallback если PowerShell недоступен
    for /f "tokens=2 delims==" %%a in ('wmic os get localdatetime /value') do set DATETIME=%%a
    set BUILD_TIME=%DATETIME:~0,4%-%DATETIME:~4,2%-%DATETIME:~6,2%T%DATETIME:~8,2%:%DATETIME:~10,2%:%DATETIME:~12,2%Z
)

REM Определение короткого хеша коммита
for /f "delims=" %%i in ('git rev-parse --short HEAD 2^>nul') do set COMMIT=%%i
if "%COMMIT%"=="" set COMMIT=unknown

echo Version: %VERSION%
echo Build time: %BUILD_TIME%
echo Commit: %COMMIT%
echo.

REM Сборка с установкой версии через ldflags
go build -ldflags "-X main.version=%VERSION% -X main.buildTime=%BUILD_TIME% -X main.commitHash=%COMMIT%" -o drupal-reminder-bot.exe main.go

if %ERRORLEVEL% EQU 0 (
    echo.
    echo Build completed successfully!
    echo Binary: drupal-reminder-bot.exe
) else (
    echo.
    echo Build failed!
    exit /b %ERRORLEVEL%
)
