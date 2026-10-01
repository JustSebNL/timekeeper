[CmdletBinding()]
param(
  [ValidateSet('install','uninstall','start','stop','status')]
  [string]$Action = 'install',
  [ValidatePattern('^[A-Za-z0-9_.-]{1,128}$')]
  [string]$Agent = 'xatia',
  [ValidatePattern('^$|^P-[0-9]+$')]
  [string]$Project = '',
  [ValidateScript({ $_ -notmatch '["\r\n]' })]
  [string]$Adapter = '',
  [ValidateRange(1,32)]
  [int]$MaxCycles = 4,
  [ValidatePattern('^[1-9][0-9]*(ms|s|m|h)$')]
  [string]$Interval = '30s'
)

$ErrorActionPreference = 'Stop'

$root = $PSScriptRoot
while ($root -and !(Test-Path -LiteralPath (Join-Path $root 'go.mod'))) {
  $root = Split-Path -Parent $root
}
if (!$root) { throw 'Cannot find TimeKeeper repository root.' }

$state = Join-Path $root '.timekeeper'
$binary = Join-Path $state 'app/bin/tk.exe'
if (!(Test-Path -LiteralPath $binary) -and $Action -notin @('uninstall','status')) {
  throw "Guardian CLI is missing: $binary"
}

$taskName = "TimeKeeper-PulseGuardian-$Agent"
$wslRoot = ($root -replace '\\','/')
if ($wslRoot -match '^([A-Za-z]):(.*)$') {
  $wslRoot = '/mnt/' + $Matches[1].ToLower() + $Matches[2]
}
$arguments = "--cd `"$wslRoot`" -- ./.timekeeper/app/bin/tk.exe guardian wake --agent $Agent --poll --continuous --until-idle --max-cycles $MaxCycles --interval $Interval"
$runner = 'wsl.exe'
if ($Adapter) {
  if (!$Project) { throw 'Native pollers require -Project P-ID to bind authorized work.' }
  if (!(Test-Path -LiteralPath $Adapter -PathType Leaf)) { throw "Adapter is missing: $Adapter" }
  $Adapter = (Resolve-Path -LiteralPath $Adapter).Path
  $runner = $binary
  $arguments = "guardian wake --agent $Agent --project $Project --adapter `"$Adapter`" --poll --continuous --until-idle --max-cycles $MaxCycles --interval $Interval"
}

function Get-GuardianTask {
  Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
}

switch ($Action) {
  'install' {
    $taskAction = New-ScheduledTaskAction -Execute $runner -Argument $arguments -WorkingDirectory $root
    $trigger = New-ScheduledTaskTrigger -AtLogOn
    $settings = New-ScheduledTaskSettingsSet `
      -RestartCount 999 `
      -RestartInterval (New-TimeSpan -Minutes 1) `
      -ExecutionTimeLimit ([TimeSpan]::Zero) `
      -StartWhenAvailable
    $principal = New-ScheduledTaskPrincipal -UserId ([Security.Principal.WindowsIdentity]::GetCurrent().Name) -LogonType Interactive -RunLevel Limited
    Register-ScheduledTask -TaskName $taskName -Action $taskAction -Trigger $trigger -Settings $settings -Principal $principal -Force | Out-Null
    Write-Output ("installed {0}: {1}" -f $taskName, $arguments)
  }
  'uninstall' {
    if (Get-GuardianTask) {
      Unregister-ScheduledTask -TaskName $taskName -Confirm:$false
      Write-Output "uninstalled $taskName"
    } else {
      Write-Output "not installed: $taskName"
    }
  }
  'start' {
    if (!(Get-GuardianTask)) { throw "Task is not installed: $taskName" }
    Start-ScheduledTask -TaskName $taskName
    Write-Output "started $taskName"
  }
  'stop' {
    if (Get-GuardianTask) {
      Stop-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
      Write-Output "stopped $taskName"
    } else {
      Write-Output "not installed: $taskName"
    }
  }
  'status' {
    $task = Get-GuardianTask
    if (!$task) {
      Write-Output "not installed: $taskName"
      exit 0
    }
    $info = Get-ScheduledTaskInfo -TaskName $taskName
    [pscustomobject]@{
      TaskName = $taskName
      State = $task.State
      LastRunTime = $info.LastRunTime
      LastTaskResult = $info.LastTaskResult
      NextRunTime = $info.NextRunTime
      Arguments = $arguments
    } | ConvertTo-Json -Compress
  }
}
