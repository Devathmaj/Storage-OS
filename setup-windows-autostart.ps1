# OS Storage Node - Windows Auto-Start Setup
# This script creates a Windows Task Scheduler entry to start the OS node on boot

$TaskName = "OS Storage Node Auto-Start"
$WSLDistro = "Ubuntu"  # Change if using different WSL distro
$StartCommand = "wsl -d $WSLDistro -u root -- systemctl start os-storage-node"

Write-Host "============================================================" -ForegroundColor Cyan
Write-Host "   OS Storage Node - Windows Auto-Start Setup" -ForegroundColor Cyan
Write-Host "============================================================" -ForegroundColor Cyan
Write-Host ""

# Check if running as Administrator
$isAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
    Write-Host "ERROR: This script must be run as Administrator" -ForegroundColor Red
    Write-Host "Right-click PowerShell and select 'Run as Administrator'" -ForegroundColor Yellow
    exit 1
}

# Check if WSL is installed
try {
    $wslVersion = wsl --version 2>$null
    if (-not $?) {
        throw "WSL not found"
    }
} catch {
    Write-Host "ERROR: WSL is not installed" -ForegroundColor Red
    Write-Host "Install WSL first: wsl --install" -ForegroundColor Yellow
    exit 1
}

# Check if the specified WSL distribution exists
$wslList = wsl -l -q
if ($wslList -notcontains $WSLDistro) {
    Write-Host "ERROR: WSL distribution '$WSLDistro' not found" -ForegroundColor Red
    Write-Host "Available distributions:" -ForegroundColor Yellow
    wsl -l
    Write-Host ""
    Write-Host "Install Ubuntu: wsl --install -d Ubuntu" -ForegroundColor Yellow
    exit 1
}

Write-Host "Creating Windows Task Scheduler entry..." -ForegroundColor Green

# Remove existing task if it exists
$existingTask = Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
if ($existingTask) {
    Write-Host "Removing existing task..." -ForegroundColor Yellow
    Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false
}

# Create the scheduled task action
$action = New-ScheduledTaskAction -Execute "wsl.exe" -Argument "-d $WSLDistro -u root -- systemctl start os-storage-node"

# Create trigger (at system startup)
$trigger = New-ScheduledTaskTrigger -AtStartup

# Create settings
$settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -StartWhenAvailable -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1)

# Create principal (run as SYSTEM with highest privileges)
$principal = New-ScheduledTaskPrincipal -UserId "SYSTEM" -LogonType ServiceAccount -RunLevel Highest

# Register the task
Register-ScheduledTask -TaskName $TaskName -Action $action -Trigger $trigger -Settings $settings -Principal $principal -Description "Automatically starts OS Storage Node on Windows boot" | Out-Null

Write-Host ""
Write-Host "============================================================" -ForegroundColor Green
Write-Host "   ✓ Auto-Start Configured Successfully!" -ForegroundColor Green
Write-Host "============================================================" -ForegroundColor Green
Write-Host ""
Write-Host "The OS Storage Node will now start automatically when Windows boots." -ForegroundColor Cyan
Write-Host ""
Write-Host "Management Commands:" -ForegroundColor Yellow
Write-Host "  Start manually:   wsl -d $WSLDistro -u root -- systemctl start os-storage-node" -ForegroundColor White
Write-Host "  Stop:             wsl -d $WSLDistro -u root -- systemctl stop os-storage-node" -ForegroundColor White
Write-Host "  Check status:     wsl -d $WSLDistro -u root -- systemctl status os-storage-node" -ForegroundColor White
Write-Host "  View logs:        wsl -d $WSLDistro -u root -- journalctl -u os-storage-node -f" -ForegroundColor White
Write-Host ""
Write-Host "Task Scheduler:" -ForegroundColor Yellow
Write-Host "  View task:        taskschd.msc" -ForegroundColor White
Write-Host "  Disable:          Disable-ScheduledTask -TaskName '$TaskName'" -ForegroundColor White
Write-Host "  Remove:           Unregister-ScheduledTask -TaskName '$TaskName'" -ForegroundColor White
Write-Host ""
Write-Host "Test it now:" -ForegroundColor Yellow
Write-Host "  Start-ScheduledTask -TaskName '$TaskName'" -ForegroundColor White
Write-Host ""
