@echo off
setlocal

if not exist build mkdir build

set APP_NAME=tts

echo Building Windows binary...
set GOOS=windows
set GOARCH=amd64
go build -o build\%APP_NAME%-windows-amd64.exe .
if errorlevel 1 goto :fail
copy /Y config.yaml build\config.yaml >nul
if errorlevel 1 goto :fail
start "" explorer "%cd%\build"

echo Building Linux binary...
set GOOS=linux
set GOARCH=amd64
go build -o build\%APP_NAME%-linux-amd64 .
if errorlevel 1 goto :fail
copy /Y config.yaml build\config.yaml >nul
if errorlevel 1 goto :fail

echo Done. Output directory: build
echo Double-click tts-windows-amd64.exe to run. Logs will be written to tts.log.
pause
exit /b 0

:fail
echo Build failed.
pause
exit /b 1
