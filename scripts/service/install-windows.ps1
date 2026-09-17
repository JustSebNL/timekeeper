[CmdletBinding()]
param(
 [ValidateSet('install','uninstall','start','stop','restart','status','logs')][string]$Action='install',
 [switch]$Build,
 [switch]$Elevate,
 [switch]$Plan,
 [string]$NssmPath=$env:TIMEKEEPER_NSSM
)
$ErrorActionPreference='Stop'
$root=$PSScriptRoot
while($root -and !(Test-Path -LiteralPath (Join-Path $root 'go.mod'))){$root=Split-Path -Parent $root}
if(!$root){throw 'Cannot find TimeKeeper repository root.'}
$state=Join-Path $root '.timekeeper'
$binary=Join-Path $state 'app/bin/timekeeper.exe'
$cliBinary=Join-Path $state 'app/bin/tk.exe'
$legacyName='TimeKeeper-Launcher'
$serviceName='TimeKeeper'
if(!$NssmPath){foreach($candidate in @((Join-Path $PSScriptRoot 'nssm/win64/nssm.exe'),(Join-Path $state 'service/nssm/nssm.exe'))){if(Test-Path -LiteralPath $candidate){$NssmPath=$candidate;break}}}
if(!$NssmPath){$found=Get-Command nssm.exe -ErrorAction SilentlyContinue;if($found){$NssmPath=$found.Source}}
if(!$NssmPath -or !(Test-Path -LiteralPath $NssmPath)){throw 'NSSM is required. Set TIMEKEEPER_NSSM to the native nssm.exe path.'}
$NssmPath=(Resolve-Path -LiteralPath $NssmPath).Path
$parameters='-addr 127.0.0.1:1618 -db "'+(Join-Path $state 'timekeeper.db')+'" -ui "'+(Join-Path $state 'web/index.html')+'" -pulse-guardian-interval 5m -proxy-addr=127.0.0.1:80'
$config=Join-Path $state 'app/INSTALLATION.env'
if(Test-Path -LiteralPath $config){
 foreach($line in Get-Content -LiteralPath $config){
  if($line -match '^TIMEKEEPER_PROXY_ADDR=(.+)$'){$parameters=$parameters -replace '-proxy-addr=\S+$',('-proxy-addr='+$Matches[1])}
  if($line -eq 'TIMEKEEPER_PROXY_DISABLED=1'){$parameters=$parameters -replace '-proxy-addr=\S+$','-proxy-addr='}
 }
}
if($Plan){[pscustomobject]@{Repository=$root;Binary=$binary;Nssm=$NssmPath;Parameters=$parameters;Service=$serviceName;Account='NT AUTHORITY\LocalService';LegacyTask='\'+$legacyName;RemoveTaskOnlyAfter='service-owned listener and TimeKeeper health verified';Build=[bool]$Build}|ConvertTo-Json;exit 0}
$identity=[Security.Principal.WindowsIdentity]::GetCurrent()
$admin=([Security.Principal.WindowsPrincipal]$identity).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if(!$admin){
 if(!$Elevate){throw 'Run the installer as Administrator, or use install.bat to request UAC elevation.'}
 $launchArgs=@('-NoProfile','-ExecutionPolicy','Bypass','-File',('"'+$PSCommandPath+'"'),'-Action',$Action,'-NssmPath',('"'+$NssmPath+'"'))
 if($Build){$launchArgs+='-Build'}
 $child=Start-Process -FilePath (Join-Path $env:SystemRoot 'System32/WindowsPowerShell/v1.0/powershell.exe') -ArgumentList $launchArgs -Verb RunAs -WindowStyle Hidden -PassThru
 $child.WaitForExit();exit $child.ExitCode
}
function Invoke-Nssm([string[]]$Arguments){$result=& $NssmPath @Arguments 2>&1;if($LASTEXITCODE -ne 0){throw "NSSM $($Arguments[0]) failed: $result"};return (($result -join "`n") -replace "`0", "").Trim()}
function Get-TkListener {
 @(Get-NetTCPConnection -LocalPort 1618 -State Listen -ErrorAction SilentlyContinue | Select-Object -ExpandProperty OwningProcess -Unique)
}
function Assert-ServiceHealth {
 $deadline=(Get-Date).AddSeconds(30)
 do {
  $svc=Get-CimInstance Win32_Service -Filter "Name='TimeKeeper'"
  if($svc -and $svc.State -eq 'Running'){
   foreach($owner in @(Get-TkListener)){
    $process=Get-CimInstance Win32_Process -Filter "ProcessId=$owner"
    if($process.ExecutablePath -ieq $binary -and $process.ParentProcessId -eq $svc.ProcessId){
     try{$health=Invoke-RestMethod 'http://127.0.0.1:1618/health' -TimeoutSec 2;$banner=Invoke-RestMethod 'http://127.0.0.1:1618/api/help' -TimeoutSec 2;if($health.status -eq 'ok' -and $banner.service -eq 'TimeKeeper'){return}}catch{}
    }
   }
  }
  Start-Sleep -Milliseconds 500
 }while((Get-Date) -lt $deadline)
 throw 'Service did not own a healthy TimeKeeper listener; legacy task retained.'
}
if($Action -eq 'status'){Get-Service -Name $serviceName;exit 0}
if($Action -eq 'logs'){Get-Content -LiteralPath (Join-Path $state 'log/service.error.log') -Tail 50;exit 0}
if($Action -in @('start','stop','restart')){Invoke-Nssm @($Action,$serviceName);if($Action -ne 'stop'){Assert-ServiceHealth};exit 0}
if($Action -eq 'uninstall'){if(Get-Service $serviceName -ErrorAction SilentlyContinue){Invoke-Nssm @('stop',$serviceName);Invoke-Nssm @('remove',$serviceName,'confirm')};exit 0}
$backup=Join-Path $state ('service/migration-'+(Get-Date -Format 'yyyyMMdd-HHmmss'))
New-Item -ItemType Directory -Path $backup -Force | Out-Null
Start-Transcript -Path (Join-Path $backup 'install.log') | Out-Null
$legacy=$null;$legacyDisabled=$false;$wasEnabled=$false;$stoppedStandalone=$false
try {
 if($Build){
  Push-Location $root
  try{foreach($name in @('timekeeper','tk')){$target=if($name -eq 'timekeeper'){'./cmd/server'}else{'./cmd/tk'};& go build -o (Join-Path $backup "$name.exe") $target;if($LASTEXITCODE -ne 0){throw "$name build failed"}}}finally{Pop-Location}
 }
 if(!(Test-Path -LiteralPath $binary) -and !$Build){throw 'Installed binaries are missing. Run install.bat to build and install.'}
 $legacy=Get-ScheduledTask -TaskName $legacyName -TaskPath '\' -ErrorAction SilentlyContinue
 if($legacy){
  $actions=@($legacy.Actions)
  if($actions.Count -ne 1 -or !(Test-Path -LiteralPath $actions[0].Execute)){throw 'Legacy task action differs from the known TimeKeeper launcher; refusing removal.'}
  $launcher=Get-Content -LiteralPath $actions[0].Execute -Raw
  if($launcher.IndexOf($root,[StringComparison]::OrdinalIgnoreCase) -lt 0 -or $launcher -notmatch 'timekeeper\.exe'){throw 'Legacy launcher does not target this repository; refusing removal.'}
  Export-ScheduledTask -TaskName $legacyName -TaskPath '\' | Set-Content -LiteralPath (Join-Path $backup 'TimeKeeper-Launcher.xml')
  $wasEnabled=$legacy.State -ne 'Disabled'
 }
 $existing=Get-Service $serviceName -ErrorAction SilentlyContinue
 if($existing){$application=Invoke-Nssm @('get',$serviceName,'Application');if($application -ine $binary){throw 'Existing TimeKeeper service belongs to a different executable; refusing takeover.'}}
 # Verify every listener's identity before stopping anything.
 foreach($owner in @(Get-TkListener)){$process=Get-CimInstance Win32_Process -Filter "ProcessId=$owner";if($process.ExecutablePath -ine $binary){throw 'Port 1618 belongs to another application.'}}
 if($legacy -and $wasEnabled){Disable-ScheduledTask -TaskName $legacyName -TaskPath '\' | Out-Null;$legacyDisabled=$true}
 if($existing -and $existing.Status -ne 'Stopped'){Invoke-Nssm @('stop',$serviceName)|Out-Null;(Get-Service $serviceName).WaitForStatus('Stopped',[TimeSpan]::FromSeconds(20))}
 foreach($owner in @(Get-TkListener)){Stop-Process -Id $owner;$stoppedStandalone=$true;Wait-Process -Id $owner -Timeout 10 -ErrorAction SilentlyContinue}
 New-Item -ItemType Directory -Path (Join-Path $state 'app/bin'),(Join-Path $state 'log'),(Join-Path $state 'web') -Force | Out-Null
 if($Build){
  foreach($name in @('timekeeper','tk')){$dest=Join-Path $state "app/bin/$name.exe";if(Test-Path -LiteralPath $dest){Copy-Item -LiteralPath $dest -Destination (Join-Path $backup "$name.previous.exe")};Copy-Item -LiteralPath (Join-Path $backup "$name.exe") -Destination $dest -Force}
  Copy-Item -Path (Join-Path $root 'web/*') -Destination (Join-Path $state 'web') -Recurse -Force
 }
 if(!(Test-Path -LiteralPath (Join-Path $state 'web/index.html'))){throw 'Dashboard assets missing.'}
 # Dedicated low-privilege service account; no user password is collected.
 & icacls.exe $state /grant '*S-1-5-19:(OI)(CI)M' /T /Q | Out-Null;if($LASTEXITCODE -ne 0){throw 'Could not grant LocalService access to TimeKeeper runtime state.'}
 if(!$existing){Invoke-Nssm @('install',$serviceName,$binary)|Out-Null}
 $settings=[ordered]@{Application=$binary;AppDirectory=$root;AppParameters=$parameters;DisplayName='TimeKeeper';Description='TimeKeeper project execution service';ObjectName='NT AUTHORITY\LocalService';Start='SERVICE_DELAYED_AUTO_START';AppStdout=(Join-Path $state 'log/service.log');AppStderr=(Join-Path $state 'log/service.error.log');AppRotateFiles='1';AppRotateBytes='10485760';AppRestartDelay='5000';AppThrottle='30000'}
 foreach($key in $settings.Keys){Invoke-Nssm @('set',$serviceName,$key,$settings[$key])|Out-Null}
 Invoke-Nssm @('set',$serviceName,'AppExit','Default','Restart')|Out-Null
 & $cliBinary hosts add;if($LASTEXITCODE -ne 0){throw 'Friendly hostname setup failed.'}
 Invoke-Nssm @('start',$serviceName)|Out-Null
 Assert-ServiceHealth
 if($legacy){Unregister-ScheduledTask -TaskName $legacyName -TaskPath '\' -Confirm:$false;if(Get-ScheduledTask -TaskName $legacyName -TaskPath '\' -ErrorAction SilentlyContinue){throw 'Legacy task removal did not persist.'}}
 [pscustomobject]@{Service='TimeKeeper';Status='Running';Health='ok';LegacyTaskRemoved=[bool]$legacy;Backup=$backup}|ConvertTo-Json | Set-Content -LiteralPath (Join-Path $state 'service/last-install.json')
 Write-Output "TimeKeeper service verified; legacy task retired. Backup: $backup"
} catch {
 if($legacyDisabled -and (Get-ScheduledTask -TaskName $legacyName -TaskPath '\' -ErrorAction SilentlyContinue)){Enable-ScheduledTask -TaskName $legacyName -TaskPath '\' | Out-Null}
 Write-Error $_
 exit 1
} finally {Stop-Transcript | Out-Null}
