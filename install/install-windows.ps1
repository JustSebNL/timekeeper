[CmdletBinding()]
param(
 [ValidateSet('install','uninstall','start','stop','restart','status','logs')][string]$Action='install',
 [switch]$Build,
 [switch]$Elevate,
 [switch]$Plan,
 [switch]$UpdateHosts,
 [switch]$RetireLegacyLauncher,
 [string]$ObeyPath=$env:TIMEKEEPER_OBEY,
 [string]$Bend2WillRoot=$env:TIMEKEEPER_BEND2WILL_ROOT
)
$ErrorActionPreference='Stop'
$ObeyPathWasProvided=[bool]$ObeyPath
$root=$PSScriptRoot
while($root -and !(Test-Path -LiteralPath (Join-Path $root 'go.mod'))){$root=Split-Path -Parent $root}
if(!$root){throw 'Cannot find TimeKeeper repository root.'}
$state=Join-Path $root '.timekeeper'
$binary=Join-Path $state 'app/bin/timekeeper.exe'
$cliBinary=Join-Path $state 'app/bin/tk.exe'
$serviceName='TimeKeeper'
$serviceDescription='Timekeeper the project manager for AI agents and human alike'
$obeyBinary=Join-Path $state 'app/bin/obey.exe'
$serviceLog=Join-Path $state 'log/service.log'
$serviceErrorLog=Join-Path $state 'log/service.error.log'
$guardianState=Join-Path $state 'app/state'
$toolsBend2Will=Join-Path $root 'tools/Bend2Will'
if(!(Test-Path -LiteralPath (Join-Path $toolsBend2Will 'go.mod'))){throw "Bend2Will source not found at $toolsBend2Will - run install.bat to sync tools/"}
$Bend2WillRoot=(Resolve-Path -LiteralPath $toolsBend2Will).Path
if($ObeyPath){if(!(Test-Path -LiteralPath $ObeyPath)){throw "obey.exe not found at $ObeyPath"};$ObeyPath=(Resolve-Path -LiteralPath $ObeyPath).Path}
if(!$ObeyPath -and (Test-Path -LiteralPath $obeyBinary)){$ObeyPath=$obeyBinary}
if(!$ObeyPath){$found=Get-Command obey.exe -ErrorAction SilentlyContinue;if($found){$ObeyPath=$found.Source}}
if(!$ObeyPath){throw 'Bend2Will obey.exe is required. Set TIMEKEEPER_OBEY or TIMEKEEPER_BEND2WILL_ROOT.'}
$existingService=Get-CimInstance Win32_Service -Filter "Name='$serviceName'" -ErrorAction SilentlyContinue
$proxyAddr='127.0.0.1:80'
$config=Join-Path $state 'app/INSTALLATION.env'
if(Test-Path -LiteralPath $config){
 foreach($line in Get-Content -LiteralPath $config){
  if($line -match '^TIMEKEEPER_PROXY_ADDR=(.+)$'){$proxyAddr=$Matches[1]}
  if($line -eq 'TIMEKEEPER_PROXY_DISABLED=1'){$proxyAddr=''}
 }
}
$historyRoot=Join-Path $state 'tracked'
if(Test-Path -LiteralPath $config){
 foreach($line in Get-Content -LiteralPath $config){
  if($line -match '^TIMEKEEPER_HISTORY_ROOT=(.+)$'){$historyRoot=$Matches[1]}
  if($line -eq 'TIMEKEEPER_HISTORY_DISABLED=1'){$historyRoot=''}
 }
}
if($historyRoot -and !(Test-Path -LiteralPath $historyRoot)){New-Item -ItemType Directory -Path $historyRoot -Force|Out-Null}
$shadowCopyDir=Join-Path (Join-Path $state 'recovery') 'sqlite-shadow'
$shadowCopyInterval='5m'
$shadowCopyDisabled=$false
if($config -and (Test-Path -LiteralPath $config)){
 foreach($line in Get-Content -LiteralPath $config){
  if($line -match '^TIMEKEEPER_SHADOW_COPY_DIR=(.+)$'){$shadowCopyDir=$Matches[1]}
  if($line -match '^TIMEKEEPER_SHADOW_COPY_INTERVAL=(.+)$'){$shadowCopyInterval=$Matches[1]}
  if($line -eq 'TIMEKEEPER_SHADOW_COPY_DISABLED=1'){$shadowCopyDisabled=$true}
 }
}
if($env:TIMEKEEPER_SHADOW_COPY_DIR){$shadowCopyDir=$env:TIMEKEEPER_SHADOW_COPY_DIR}
if($env:TIMEKEEPER_SHADOW_COPY_INTERVAL){$shadowCopyInterval=$env:TIMEKEEPER_SHADOW_COPY_INTERVAL}
if($env:TIMEKEEPER_SHADOW_COPY_DISABLED -eq '1'){$shadowCopyDisabled=$true}
if($shadowCopyDisabled){$shadowCopyInterval='0'}
$serviceArgsArray=@('-addr','timekeeper.local','-db',(Join-Path $state 'timekeeper.db'),'-ui',(Join-Path $state 'web/index.html'),'-pulse-guardian-interval','5m','-guardian-state-dir',$guardianState,'-shadow-copy-dir',$shadowCopyDir,'-shadow-copy-interval',$shadowCopyInterval,('-proxy-addr='+$proxyAddr))
if($historyRoot){$serviceArgsArray+=('-history-root='+$historyRoot)}
$parameters=$serviceArgsArray -join [char]31
$parametersForDisplay=ConvertTo-Json -InputObject $serviceArgsArray -Compress
if($Plan){
 [pscustomobject]@{
  Repository=$root;Binary=$binary;Obey=$ObeyPath;ObeyTarget=$obeyBinary;Bend2WillRoot=$Bend2WillRoot;ServiceDescription=$serviceDescription
  Parameters=$parametersForDisplay;Service=$serviceName;Manager='Bend2Will obey'
  Account='NT AUTHORITY\LocalService';Start='delayed-auto';RestartOnFailure='5s';Log=$serviceLog;LogMaxBytes=10485760
  UpdateHosts=[bool]$UpdateHosts;Build=[bool]$Build
 }|ConvertTo-Json
 exit 0
}
$privilegedActions=@('install','uninstall','start','stop','restart')
if($privilegedActions -contains $Action){
$identity=[Security.Principal.WindowsIdentity]::GetCurrent()
$admin=([Security.Principal.WindowsPrincipal]$identity).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if(!$admin){
 if(!$Elevate){throw 'Run the installer as Administrator, or use install.bat to request UAC elevation.'}
 $launchArgs=@('-NoProfile','-ExecutionPolicy','Bypass','-File',('\"'+$PSCommandPath+'\"'),'-Action',$Action,'-Elevate')
 if($Build){$launchArgs+='-Build'}
 if($UpdateHosts){$launchArgs+='-UpdateHosts'}
 if($RetireLegacyLauncher){$launchArgs+='-RetireLegacyLauncher'}
 if($ObeyPathWasProvided){$launchArgs+=@('-ObeyPath',('\"'+$ObeyPath+'\"'))}
 if($Bend2WillRoot){$launchArgs+=@('-Bend2WillRoot',('\"'+$Bend2WillRoot+'\"'))}
 $child=Start-Process -FilePath (Join-Path $env:SystemRoot 'System32/WindowsPowerShell/v1.0/powershell.exe') -ArgumentList $launchArgs -Verb RunAs -WindowStyle Hidden -PassThru
  $child.WaitForExit();exit $child.ExitCode
 }
}
function Invoke-Obey([string]$Executable,[string[]]$Arguments){if(!$Executable -or !(Test-Path -LiteralPath $Executable)){throw 'Bend2Will obey.exe is unavailable.'};$result=& $Executable @Arguments 2>&1;if($LASTEXITCODE -ne 0){throw "obey $($Arguments[0]) failed: $result"};return (($result -join \"`n\") -replace \"`0\", \"\").Trim()}
function Wait-ServiceRemoved {
 $deadline=(Get-Date).AddSeconds(30)
 do{if(!(Get-CimInstance Win32_Service -Filter \"Name='$serviceName'\" -ErrorAction SilentlyContinue)){return};Start-Sleep -Milliseconds 200}while((Get-Date) -lt $deadline)
 throw \"Windows did not finish removing service '$serviceName'.\"
}
function Install-ObeyService([string]$ArgumentsEncoded,[string]$LogPath,[long]$LogMaxBytes,[string]$Description=$serviceDescription){
 Wait-ServiceRemoved
 $arguments=@('install',$serviceName,'--command',$binary,'--arguments',$ArgumentsEncoded,'--workdir',$root,'--description',$Description,'--log',$LogPath,'--log-max-bytes',\"$LogMaxBytes\",'--service-account','LocalService','--enable','--delayed-auto-start','--restart-service-on-failure','--service-restart-delay','5s','--now')
 Invoke-Obey $obeyBinary $arguments|Out-Null
}
function Build-Obey([string]$Destination){
 if(!$Bend2WillRoot){throw 'Bend2Will source is unavailable; set TIMEKEEPER_BEND2WILL_ROOT or pass -ObeyPath.'}
 Push-Location $Bend2WillRoot
 try{& go build -o $Destination ./cmd/obey;if($LASTEXITCODE -ne 0){throw 'Bend2Will obey build failed'}}finally{Pop-Location}
 return $Destination
}
function Get-ServiceExecutablePath([string]$PathName){if($PathName -match '^\s*\"([^\"]+)\"'){return $Matches[1]};if($PathName -match '^\s*(\S+)'){return $Matches[1]};return ''}
function Get-ServiceBackend($Service){if(!$Service){return 'none'};$exe=[IO.Path]::GetFileName((Get-ServiceExecutablePath $Service.PathName));if($exe -ieq 'obey.exe'){return 'obey'};return 'unknown'}
function Get-TkListener {
 @(Get-NetTCPConnection -LocalPort 1618 -State Listen -ErrorAction SilentlyContinue | Select-Object -ExpandProperty OwningProcess -Unique)
}
function Assert-ServiceHealth {
 $deadline=(Get-Date).AddSeconds(30)
 do {
  $svc=Get-CimInstance Win32_Service -Filter \"Name='TimeKeeper'\"
  if($svc -and $svc.State -eq 'Running'){
   foreach($owner in @(Get-TkListener)){
    $process=Get-CimInstance Win32_Process -Filter \"ProcessId=$owner\"
    if($process.ExecutablePath -ieq $binary -and $process.ParentProcessId -eq $svc.ProcessId){
     try{$health=Invoke-RestMethod 'http://timekeeper.local/health' -TimeoutSec 2;$banner=Invoke-RestMethod 'http://timekeeper.local/api/help' -TimeoutSec 2;if($health.status -eq 'ok' -and $banner.service -eq 'TimeKeeper'){return}}catch{}
    }
   }
  }
  Start-Sleep -Milliseconds 500
 }while((Get-Date) -lt $deadline)
 throw 'Service did not own a healthy TimeKeeper listener; the installation cannot be considered successful.'
}
$existingService=Get-CimInstance Win32_Service -Filter "Name='$serviceName'"
$existingBackend=Get-ServiceBackend $existingService
$serviceObeyPath=if($existingBackend -eq 'obey'){$serviceExecutable}else{$ObeyPath}
if($Action -eq 'status'){
 if($existingBackend -eq 'obey'){Invoke-Obey $serviceObeyPath @('report',$serviceName);exit 0}
 if($existingBackend -eq 'none'){Write-Output "Service '$serviceName' is not installed.";exit 1}
 throw "Refusing to manage unknown service image: $($existingService.PathName)"
}
if($Action -eq 'logs'){
 foreach($path in @($serviceLog+'.1',$serviceLog,$serviceErrorLog)){if(Test-Path -LiteralPath $path){Get-Content -LiteralPath $path -Tail 50}}
 exit 0
}
if($Action -in @('start','stop','restart')){
 if($existingBackend -eq 'obey'){Invoke-Obey $serviceObeyPath @($Action,$serviceName)|Out-Null}
 elseif($existingBackend -eq 'none'){throw "Service '$serviceName' is not installed."}
 else{throw "Refusing to manage unknown service image: $($existingService.PathName)"}
 if($Action -ne 'stop'){Assert-ServiceHealth}
 exit 0
}
if($Action -eq 'uninstall'){
 if($existingBackend -eq 'obey'){Invoke-Obey $serviceObeyPath @('release',$serviceName)|Out-Null}
 exit 0
}
if($Action -eq 'install'){
 $backup=Join-Path $state ('service/migration-'+(Get-Date -Format 'yyyyMMdd-HHmmss'))
 New-Item -ItemType Directory -Path $backup -Force | Out-Null
 Start-Transcript -Path (Join-Path $backup 'install.log') | Out-Null
 try {
  if($Build){
   Push-Location $root
   try{foreach($name in @('timekeeper','tk')){$target=if($name -eq 'timekeeper'){'./cmd/server'}else{'./cmd/tk'};& go build -o (Join-Path $backup "$name.exe") $target;if($LASTEXITCODE -ne 0){throw "$name build failed"}}}finally{Pop-Location}
  }
  if(!(Test-Path -LiteralPath $binary) -and !$Build){throw 'Installed binaries are missing. Run install.bat to build and install.'}
  $obeySource=$null
  if($ObeyPathWasProvided){$obeySource=$ObeyPath}
  elseif($Build -and $Bend2WillRoot){$obeySource=Build-Obey (Join-Path $backup 'obey.exe')}
  elseif(Test-Path -LiteralPath $obeyBinary){$obeySource=$obeyBinary}
  elseif($ObeyPath){$obeySource=$ObeyPath}
  elseif($Bend2WillRoot){$obeySource=Build-Obey (Join-Path $backup 'obey.exe')}
  if(!$obeySource -or !(Test-Path -LiteralPath $obeySource)){throw 'Bend2Will obey.exe is required. Set TIMEKEEPER_OBEY or TIMEKEEPER_BEND2WILL_ROOT.'}
  $existingBackend=if($existingService){Get-ServiceBackend $existingService}else{'none'}
  if($existingBackend -eq 'unknown'){throw "Refusing to replace unknown service image: $($existingService.PathName)"}
  $legacy=Get-ScheduledTask -TaskName 'TimeKeeper-Launcher' -TaskPath '\' -ErrorAction SilentlyContinue
  if($legacy){
   $actions=@($legacy.Actions)
   if($actions.Count -ne 1 -or !(Test-Path -LiteralPath $actions[0].Execute)){throw 'Legacy task action differs from the known TimeKeeper launcher; refusing removal.'}
   $launcher=Get-Content -LiteralPath $actions[0].Execute -Raw
   if($launcher.IndexOf($root,[StringComparison]::OrdinalIgnoreCase) -lt 0 -or $launcher -notmatch 'timekeeper\.exe'){throw 'Legacy launcher does not target this repository; refusing removal.'}
   Export-ScheduledTask -TaskName 'TimeKeeper-Launcher' -TaskPath '\' | Set-Content -LiteralPath (Join-Path $backup 'TimeKeeper-Launcher.xml')
   $wasEnabled=$legacy.State -ne 'Disabled'
  }
  foreach($owner in @(Get-TkListener)){$process=Get-CimInstance Win32_Process -Filter "ProcessId=$owner";if($process.ExecutablePath -ine $binary){throw 'Port 1618 belongs to another application.'}}
  if($existingService -and $existingService.State -ne 'Stopped'){
   $serviceExecutable=Get-ServiceExecutablePath $existingService.PathName
   Invoke-Obey $serviceExecutable @('stop',$serviceName)|Out-Null
   (Get-Service $serviceName).WaitForStatus('Stopped',[TimeSpan]::FromSeconds(30))
  }
  foreach($owner in @(Get-TkListener)){Stop-Process -Id $owner;Wait-Process -Id $owner -Timeout 10 -ErrorAction SilentlyContinue}
  New-Item -ItemType Directory -Path (Join-Path $state 'app/bin'),$guardianState,(Join-Path $state 'log'),(Join-Path $state 'web') -Force | Out-Null
  if($obeySource -ine $obeyBinary){if(Test-Path -LiteralPath $obeyBinary){Copy-Item -LiteralPath $obeyBinary -Destination (Join-Path $backup 'obey.previous.exe')} ; Copy-Item -LiteralPath $obeySource -Destination $obeyBinary -Force}
  if($Build){
   foreach($name in @('timekeeper','tk')){
    $dest=Join-Path $state "app/bin/$name.exe"
    if(Test-Path -LiteralPath $dest){Copy-Item -LiteralPath $dest -Destination (Join-Path $backup "$name.previous.exe")}
    Copy-Item -LiteralPath (Join-Path $backup "$name.exe") -Destination $dest -Force
   }
   $webState=Join-Path $state 'web';$webBackup=Join-Path $backup 'web.previous'
   if(Test-Path -LiteralPath $webState){Copy-Item -LiteralPath $webState -Destination $webBackup -Recurse}
   Copy-Item -Path (Join-Path $root 'web/*') -Destination $webState -Recurse -Force
  }
  if(!(Test-Path -LiteralPath (Join-Path $state 'web/index.html'))){throw 'Dashboard assets missing.'}
  & icacls.exe $state /grant '*S-1-5-19:(OI)(CI)M' /T /Q | Out-Null;if($LASTEXITCODE -ne 0){throw 'Could not grant LocalService access to TimeKeeper runtime state.'}
  if($shadowCopyInterval -ne '0'){
   try{
    New-Item -ItemType Directory -Path $shadowCopyDir -Force|Out-Null
    $shadowUserSid=[Security.Principal.WindowsIdentity]::GetCurrent().User.Value
    $shadowAclArgs=@($shadowCopyDir,'/inheritance:r','/remove:g','*S-1-1-0','/remove:d','*S-1-1-0','/grant','*S-1-5-18:(OI)(CI)F','*S-1-5-19:(OI)(CI)M','*S-1-5-32-544:(OI)(CI)F',('*'+$shadowUserSid+':(OI)(CI)F'),'/T','/Q')
    & icacls.exe @shadowAclArgs|Out-Null
    if($LASTEXITCODE -ne 0){throw "Could not secure shadow directory $shadowCopyDir."}
   }catch{throw "Cannot prepare secure shadow directory $shadowCopyDir : $_"}
  }
  if($existingService){Invoke-Obey (Get-ServiceExecutablePath $existingService.PathName) @('release',$serviceName)|Out-Null}
  Wait-ServiceRemoved
  Install-ObeyService $parameters $serviceLog 10485760
  Assert-ServiceHealth
  if($RetireLegacyLauncher -and $legacy){Unregister-ScheduledTask -TaskName 'TimeKeeper-Launcher' -TaskPath '\' -Confirm:$false}
  [pscustomobject]@{ServiceManager='Bend2Will';Service=$serviceName;Status='Running';Health='ok';ObeyPath=$obeyBinary;Parameters=$parameters;ServiceDescription=$serviceDescription;LogPath=$serviceLog;LogMaxBytes=10485760;Account='NT AUTHORITY\LocalService';Start='delayed-auto';RestartServiceOnFailure=$true;ServiceRestartDelay='5s';LegacyTaskRemoved=[bool]($legacy -and $RetireLegacyLauncher);LegacyTaskRetained=[bool]($legacy -and !$RetireLegacyLauncher);Backup=$backup}|ConvertTo-Json -Depth 4 | Set-Content -LiteralPath (Join-Path $state 'service/last-install.json')
  Write-Output "TimeKeeper service verified; backup: $backup"
 } catch {
  $failure=$_
  Write-Error $failure
  exit 1
 } finally {Stop-Transcript | Out-Null}
}
