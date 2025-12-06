@echo off
REM Quick setup script for Windows users
REM This installs WSL, OS Storage Node, and sets up auto-start

echo ============================================================
echo    OS Storage Node - Windows Quick Setup
echo ============================================================
echo.

REM Check if running as Administrator
net session >nul 2>&1
if %errorLevel% neq 0 (
    echo ERROR: This script must be run as Administrator
    echo Right-click this file and select "Run as administrator"
    pause
    exit /b 1
)

echo Step 1: Checking WSL installation...
wsl --version >nul 2>&1
if %errorLevel% neq 0 (
    echo WSL not found. Installing WSL...
    wsl --install -d Ubuntu
    echo.
    echo WSL installed! Please restart your computer, then run this script again.
    pause
    exit /b 0
)

echo ✓ WSL is installed
echo.

echo Step 2: Checking if OS Storage Node is installed in WSL...
wsl -d Ubuntu -u root -- test -f /opt/os-storage-node/bin/os-storage-node
if %errorLevel% neq 0 (
    echo.
    echo OS Storage Node not found in WSL.
    echo.
    echo Please complete these steps in Ubuntu WSL terminal:
    echo   1. Open Ubuntu from Start Menu
    echo   2. Run: sudo apt-get update
    echo   3. Run: sudo apt-get install -y qemu-system-x86
    echo   4. Extract and install the OS Storage Node package
    echo   5. Run this script again
    echo.
    pause
    exit /b 1
)

echo ✓ OS Storage Node is installed
echo.

echo Step 3: Starting OS Storage Node service...
wsl -d Ubuntu -u root -- systemctl start os-storage-node
wsl -d Ubuntu -u root -- systemctl enable os-storage-node

echo ✓ Service started
echo.

echo Step 4: Setting up Windows auto-start...

REM Create scheduled task
schtasks /query /TN "OS Storage Node Auto-Start" >nul 2>&1
if %errorLevel% equ 0 (
    echo Removing existing task...
    schtasks /delete /TN "OS Storage Node Auto-Start" /F >nul
)

echo Creating scheduled task...
schtasks /create /TN "OS Storage Node Auto-Start" /TR "wsl.exe -d Ubuntu -u root -- systemctl start os-storage-node" /SC ONSTART /RU SYSTEM /RL HIGHEST /F >nul

echo.
echo ============================================================
echo    ✓ Setup Complete!
echo ============================================================
echo.
echo OS Storage Node is now running and will auto-start with Windows.
echo.
echo Access the node at: http://localhost:10082
echo.
echo Management commands (run in PowerShell or CMD):
echo   Check status:  wsl -d Ubuntu -u root -- systemctl status os-storage-node
echo   Stop service:  wsl -d Ubuntu -u root -- systemctl stop os-storage-node
echo   View logs:     wsl -d Ubuntu -u root -- journalctl -u os-storage-node -f
echo.
pause
