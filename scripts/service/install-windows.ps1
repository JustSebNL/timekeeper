[CmdletBinding()]
param(
 [ValidateSet('install','uninstall','start','stop','restart','status','logs')][string]$Action='install',
 [switch]$Build,
 [switch]$Elevate,
 [switch]$Plan,
 [switch]$MigrateFromNssm,
 [switch]$UpdateHosts,
 [switch]$RetireLegacyLauncher,
 [string]$ObeyPath=$env:TIMEKEEPER_OBEY,
 [string]$Bend2WillRoot=$env:TIMEKEEPER_BEND2WILL_ROOT,
 [string]$NssmPath=$env:TIMEKEEPER_NSSM
)
$ErrorActionPreference='Stop'
$ObeyPathWasProvided=[bool]$ObeyPath
$root=$PSScriptRoot
while($root -and !(Test-Path -LiteralPath (Join-Path $root 'go.mod'))){$root=Split-Path -Parent $root}
if(!$root){throw 'Cannot find TimeKeeper repository root.'}
$state=Join-Path $root '.timekeeper'
$binary=Join-Path $state 'app/bin/timekeeper.exe'
$cliBinary=Join-Path $state 'app/bin/tk.exe'
$legacyName='TimeKeeper-Launcher'
$serviceName='TimeKeeper'
$obeyBinary=Join-Path $state 'app/bin/obey.exe'
$serviceLog=Join-Path $state 'log/service.log'
$serviceErrorLog=Join-Path $state 'log/service.error.log'
if(!$Bend2WillRoot){$candidate=Join-Path (Split-Path $root -Parent) 'Bend2Will';if(Test-Path -LiteralPath (Join-Path $candidate 'go.mod')){$Bend2WillRoot=$candidate}}
if($Bend2WillRoot){if(!(Test-Path -LiteralPath (Join-Path $Bend2WillRoot 'go.mod'))){throw "Bend2Will source not found at $Bend2WillRoot"};$Bend2WillRoot=(Resolve-Path -LiteralPath $Bend2WillRoot).Path}
if($ObeyPath){if(!(Test-Path -LiteralPath $ObeyPath)){throw "obey.exe not found at $ObeyPath"};$ObeyPath=(Resolve-Path -LiteralPath $ObeyPath).Path}
if(!$ObeyPath -and (Test-Path -LiteralPath $obeyBinary)){$ObeyPath=$obeyBinary}
if(!$ObeyPath){$found=Get-Command obey.exe -ErrorAction SilentlyContinue;if($found){$ObeyPath=$found.Source}}
if(!$NssmPath){foreach($candidate in @((Join-Path $PSScriptRoot 'nssm/win64/nssm.exe'),(Join-Path $state 'service/nssm/nssm.exe'))){if(Test-Path -LiteralPath $candidate){$NssmPath=$candidate;break}}}
if(!$NssmPath){$found=Get-Command nssm.exe -ErrorAction SilentlyContinue;if($found){$NssmPath=$found.Source}}
if($NssmPath -and (Test-Path -LiteralPath $NssmPath)){$NssmPath=(Resolve-Path -LiteralPath $NssmPath).Path}else{$NssmPath=$null}
$installManager=if($ObeyPath -or $Bend2WillRoot){'Bend2Will obey'}elseif($NssmPath){'NSSM legacy fallback'}else{'unavailable'}
$existingPlanService=Get-CimInstance Win32_Service -Filter "Name='$serviceName'" -ErrorAction SilentlyContinue
$existingPlanBackend='none'
if($existingPlanService){if($existingPlanService.PathName -match '(?i)obey\.exe'){$existingPlanBackend='Bend2Will'}elseif($existingPlanService.PathName -match '(?i)nssm\.exe'){$existingPlanBackend='NSSM'}else{$existingPlanBackend='unknown'}}
$existingPlanLegacy=Get-ScheduledTask -TaskName 'TimeKeeper-Launcher' -TaskPath '\' -ErrorAction SilentlyContinue
$proxyAddr='127.0.0.1:80'
$config=Join-Path $state 'app/INSTALLATION.env'
if(Test-Path -LiteralPath $config){
 foreach($line in Get-Content -LiteralPath $config){
  if($line -match '^TIMEKEEPER_PROXY_ADDR=(.+)$'){$proxyAddr=$Matches[1]}
  if($line -eq 'TIMEKEEPER_PROXY_DISABLED=1'){$proxyAddr=''}
 }
}
$serviceArgsArray=@('-addr','127.0.0.1:1618','-db',(Join-Path $state 'timekeeper.db'),'-ui',(Join-Path $state 'web/index.html'),'-pulse-guardian-interval','5m',('-proxy-addr='+$proxyAddr))
$parameters=$serviceArgsArray -join [char]31
$parametersForDisplay=ConvertTo-Json -InputObject $serviceArgsArray -Compress
$nssmParameters='-addr 127.0.0.1:1618 -db "'+(Join-Path $state 'timekeeper.db')+'" -ui "'+(Join-Path $state 'web/index.html')+'" -pulse-guardian-interval 5m -proxy-addr='+$proxyAddr
if($Plan){
 [pscustomobject]@{
  Repository=$root;Binary=$binary;Obey=$ObeyPath;ObeyTarget=$obeyBinary;Bend2WillRoot=$Bend2WillRoot
  Parameters=$parametersForDisplay;NssmParameters=$nssmParameters;Service=$serviceName;Manager=$installManager
  ExistingServiceBackend=$existingPlanBackend;MigrationRequired=($existingPlanBackend -eq 'NSSM' -and $installManager -eq 'Bend2Will obey')
  Account='NT AUTHORITY\LocalService';Start='delayed-auto';RestartOnFailure='5s';Log=$serviceLog;LogMaxBytes=10485760
  LegacyTask='\'+$legacyName;LegacyTaskPresent=[bool]$existingPlanLegacy;RetireLegacyLauncher=[bool]$RetireLegacyLauncher
  UpdateHosts=[bool]$UpdateHosts;MigrateFromNssm=[bool]$MigrateFromNssm;Build=[bool]$Build
 }|ConvertTo-Json
 exit 0
}
if($Action -eq 'install' -and $installManager -eq 'Bend2Will obey' -and $existingPlanBackend -eq 'NSSM' -and !$MigrateFromNssm){throw 'An NSSM TimeKeeper service is installed. No service was changed; rerun with -MigrateFromNssm to approve the Bend2Will migration.'}
$identity=[Security.Principal.WindowsIdentity]::GetCurrent()
$admin=([Security.Principal.WindowsPrincipal]$identity).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if(!$admin){
 if(!$Elevate){throw 'Run the installer as Administrator, or use install.bat to request UAC elevation.'}
 $launchArgs=@('-NoProfile','-ExecutionPolicy','Bypass','-File',('"'+$PSCommandPath+'"'),'-Action',$Action,'-Elevate')
 if($Build){$launchArgs+='-Build'}
 if($MigrateFromNssm){$launchArgs+='-MigrateFromNssm'}
 if($UpdateHosts){$launchArgs+='-UpdateHosts'}
 if($RetireLegacyLauncher){$launchArgs+='-RetireLegacyLauncher'}
 if($ObeyPathWasProvided){
$launchArgs+=@('-ObeyPath',('"'+$ObeyPath+'"'))}
 if($Bend2WillRoot){$launchArgs+=@('-Bend2WillRoot',('"'+$Bend2WillRoot+'"'))}
 if($NssmPath){$launchArgs+=@('-NssmPath',('"'+$NssmPath+'"'))}
 $child=Start-Process -FilePath (Join-Path $env:SystemRoot 'System32/WindowsPowerShell/v1.0/powershell.exe') -ArgumentList $launchArgs -Verb RunAs -WindowStyle Hidden -PassThru
 $child.WaitForExit();exit $child.ExitCode
}
function Invoke-Nssm([string[]]$Arguments){if(!$NssmPath -or !(Test-Path -LiteralPath $NssmPath)){throw 'NSSM is unavailable for the installed legacy service.'};$result=& $NssmPath @Arguments 2>&1;if($LASTEXITCODE -ne 0){throw "NSSM $($Arguments[0]) failed: $result"};return (($result -join "`n") -replace "`0", "").Trim()}
function Invoke-NssmStartOrRestart([string]$Action){
 $result=& $NssmPath $Action $serviceName 2>&1
 if($LASTEXITCODE -ne 0){$service=Get-CimInstance Win32_Service -Filter "Name='$serviceName'" -ErrorAction SilentlyContinue;if(!$service -or ($service.State -ne 'Running' -and $service.State -notmatch 'Pending')){throw "NSSM $Action failed: $result"}}
 (Get-Service -Name $serviceName -ErrorAction Stop).WaitForStatus('Running',[TimeSpan]::FromSeconds(30))
}
function Get-NssmSettings {
 $settings=[ordered]@{}
 foreach($key in @('Application','AppDirectory','AppParameters','DisplayName','Description','ObjectName','Start','AppStdout','AppStderr','AppStdoutCreationDisposition','AppStderrCreationDisposition','AppRotateFiles','AppRotateBytes','AppRestartDelay','AppThrottle')){
  $value=& $NssmPath get $serviceName $key 2>$null
  if($LASTEXITCODE -eq 0){$settings[$key]=(($value -join "`n") -replace "`0", "").Trim()}
 }
 $appExit=& $NssmPath get $serviceName AppExit Default 2>$null
 if($LASTEXITCODE -eq 0){$settings['AppExitDefault']=(($appExit -join "`n") -replace "`0", "").Trim()}
 if(!$settings.Application){throw 'Cannot back up the existing NSSM service configuration.'}
 return $settings
}
function Apply-NssmSettings($Settings){
 foreach($key in $Settings.Keys){
  if($key -eq 'Application' -or $key -eq 'AppExitDefault'){continue}
  if($Settings[$key]){Invoke-Nssm @('set',$serviceName,$key,$Settings[$key])|Out-Null}
 }
 if($Settings.AppExitDefault){Invoke-Nssm @('set',$serviceName,'AppExit','Default',$Settings.AppExitDefault)|Out-Null}
}
function Restore-NssmSettings($Settings){
 Wait-ServiceRemoved
 Invoke-Nssm @('install',$serviceName,$Settings.Application)|Out-Null
 Apply-NssmSettings $Settings
}
function Invoke-Obey([string]$Executable,[string[]]$Arguments){if(!$Executable -or !(Test-Path -LiteralPath $Executable)){throw 'Bend2Will obey.exe is unavailable.'};$result=& $Executable @Arguments 2>&1;if($LASTEXITCODE -ne 0){throw "obey $($Arguments[0]) failed: $result"};return (($result -join "`n") -replace "`0", "").Trim()}
function Wait-ServiceRemoved {
 $deadline=(Get-Date).AddSeconds(30)
 do{if(!(Get-CimInstance Win32_Service -Filter "Name='$serviceName'" -ErrorAction SilentlyContinue)){return};Start-Sleep -Milliseconds 200}while((Get-Date) -lt $deadline)
 throw "Windows did not finish removing service '$serviceName'."
}
function Install-ObeyService([string]$ArgumentsEncoded,[string]$LogPath,[long]$LogMaxBytes){
 Wait-ServiceRemoved
 $arguments=@('install',$serviceName,'--command',$binary,'--arguments',$ArgumentsEncoded,'--workdir',$root,'--log',$LogPath,'--log-max-bytes',"$LogMaxBytes",'--service-account','LocalService','--enable','--delayed-auto-start','--restart-service-on-failure','--service-restart-delay','5s','--now')
 Invoke-Obey $obeyBinary $arguments|Out-Null
}
function Build-Obey([string]$Destination){
 if(!$Bend2WillRoot){throw 'Bend2Will source is unavailable; set TIMEKEEPER_BEND2WILL_ROOT or pass -ObeyPath.'}
 Push-Location $Bend2WillRoot
 try{& go build -o $Destination ./cmd/obey;if($LASTEXITCODE -ne 0){throw 'Bend2Will obey build failed'}}finally{Pop-Location}
 return $Destination
}
function Get-ServiceExecutablePath([string]$PathName){if($PathName -match '^\s*"([^"]+)"'){return $Matches[1]};if($PathName -match '^\s*(\S+)'){return $Matches[1]};return ''}
function Get-ServiceBackend($Service){if(!$Service){return 'none'};$exe=[IO.Path]::GetFileName((Get-ServiceExecutablePath $Service.PathName));if($exe -ieq 'obey.exe'){return 'obey'};if($exe -ieq 'nssm.exe'){return 'nssm'};return 'unknown'}
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
 throw 'Service did not own a healthy TimeKeeper listener; the installation cannot be considered successful.'
}
$existingService=Get-CimInstance Win32_Service -Filter "Name='$serviceName'" -ErrorAction SilentlyContinue
$existingBackend=Get-ServiceBackend $existingService
$serviceExecutable=if($existingService){Get-ServiceExecutablePath $existingService.PathName}else{''}
if($existingBackend -eq 'nssm' -and !$NssmPath){$NssmPath=$serviceExecutable}
if($existingBackend -eq 'obey'){$serviceObeyPath=$serviceExecutable}else{$serviceObeyPath=$ObeyPath}
if($Action -eq 'status'){
 if($existingBackend -eq 'obey'){Invoke-Obey $serviceObeyPath @('report',$serviceName);exit 0}
 if($existingBackend -eq 'nssm'){Invoke-Nssm @('status',$serviceName);exit 0}
 if($existingBackend -eq 'none'){Write-Output "Service '$serviceName' is not installed.";exit 1}
 throw "Refusing to manage unknown service image: $($existingService.PathName)"
}
if($Action -eq 'logs'){
 foreach($path in @($serviceLog+'.1',$serviceLog,$serviceErrorLog)){if(Test-Path -LiteralPath $path){Get-Content -LiteralPath $path -Tail 50}}
 exit 0
}
if($Action -in @('start','stop','restart')){
 if($existingBackend -eq 'obey'){Invoke-Obey $serviceObeyPath @($Action,$serviceName)|Out-Null}
 elseif($existingBackend -eq 'nssm'){
  if($Action -eq 'stop'){Invoke-Nssm @('stop',$serviceName)|Out-Null;(Get-Service -Name $serviceName).WaitForStatus('Stopped',[TimeSpan]::FromSeconds(30))}
  else{Invoke-NssmStartOrRestart $Action}
 }
 else{throw "Service '$serviceName' is not installed by a supported manager."}
 if($Action -ne 'stop'){Assert-ServiceHealth}
 exit 0
}
if($Action -eq 'uninstall'){
 if($existingBackend -eq 'obey'){Invoke-Obey $serviceObeyPath @('release',$serviceName)|Out-Null}
 elseif($existingBackend -eq 'nssm'){Invoke-Nssm @('stop',$serviceName)|Out-Null;Invoke-Nssm @('remove',$serviceName,'confirm')|Out-Null}
 elseif($existingBackend -eq 'unknown'){throw "Refusing to remove unknown service image: $($existingService.PathName)"}
 exit 0
}
$backup=Join-Path $state ('service/migration-'+(Get-Date -Format 'yyyyMMdd-HHmmss'))
New-Item -ItemType Directory -Path $backup -Force | Out-Null
Start-Transcript -Path (Join-Path $backup 'install.log') | Out-Null
$legacy=$null;$legacyDisabled=$false;$wasEnabled=$false;$stoppedStandalone=$false
$legacyNssmSettings=$null;$legacyNssmRemoved=$false;$nssmInstalled=$false;$obeyInstalled=$false;$newObeyAttempted=$false;$obeyServiceRemoved=$false;$serviceAlreadyConfigured=$false;$legacyWasRunning=$false
$previousBinarySaved=$false;$previousTkSaved=$false;$previousObeySaved=$false;$previousWebSaved=$false;$previousObeyMetadata=$null;$previousInstallRecordSaved=$false
$installRecordPath=Join-Path $state 'service/last-install.json'
$previousInstallRecordPath=Join-Path $backup 'last-install.previous.json'
if(Test-Path -LiteralPath $installRecordPath){Copy-Item -LiteralPath $installRecordPath -Destination $previousInstallRecordPath;$previousInstallRecordSaved=$true}
try {
 if($Build){
  Push-Location $root
  try{foreach($name in @('timekeeper','tk')){$target=if($name -eq 'timekeeper'){'./cmd/server'}else{'./cmd/tk'};& go build -o (Join-Path $backup "$name.exe") $target;if($LASTEXITCODE -ne 0){throw "$name build failed"}}}finally{Pop-Location}
 }
 if(!(Test-Path -LiteralPath $binary) -and !$Build){throw 'Installed binaries are missing. Run install.bat to build and install.'}
 $obeySource=$null
if($installManager -eq 'Bend2Will obey'){
 if($ObeyPathWasProvided){$obeySource=$ObeyPath}
 elseif($Build -and $Bend2WillRoot){$obeySource=Build-Obey (Join-Path $backup 'obey.exe')}
 elseif(Test-Path -LiteralPath $obeyBinary){$obeySource=$obeyBinary}
 elseif($ObeyPath){$obeySource=$ObeyPath}
 elseif($Bend2WillRoot){$obeySource=Build-Obey (Join-Path $backup 'obey.exe')}
 if(!$obeySource -or !(Test-Path -LiteralPath $obeySource)){throw 'Bend2Will obey.exe is required. Set TIMEKEEPER_OBEY or TIMEKEEPER_BEND2WILL_ROOT.'}
}elseif($installManager -eq 'unavailable'){
 throw 'No supported Windows service manager was found. Provide Bend2Will obey or NSSM.'
}
$legacy=Get-ScheduledTask -TaskName $legacyName -TaskPath '\' -ErrorAction SilentlyContinue
 if($legacy){
  $actions=@($legacy.Actions)
  if($actions.Count -ne 1 -or !(Test-Path -LiteralPath $actions[0].Execute)){throw 'Legacy task action differs from the known TimeKeeper launcher; refusing removal.'}
  $launcher=Get-Content -LiteralPath $actions[0].Execute -Raw
  if($launcher.IndexOf($root,[StringComparison]::OrdinalIgnoreCase) -lt 0 -or $launcher -notmatch 'timekeeper\.exe'){throw 'Legacy launcher does not target this repository; refusing removal.'}
  Export-ScheduledTask -TaskName $legacyName -TaskPath '\' | Set-Content -LiteralPath (Join-Path $backup 'TimeKeeper-Launcher.xml')
  $wasEnabled=$legacy.State -ne 'Disabled'
 }
  if($existingBackend -eq 'nssm'){
 if($installManager -eq 'Bend2Will obey' -and !$MigrateFromNssm){throw 'An NSSM TimeKeeper service is installed. No service was changed; rerun with -MigrateFromNssm to approve the Bend2Will migration.'}
 if($MigrateFromNssm -and $installManager -ne 'Bend2Will obey'){throw 'NSSM migration requires Bend2Will obey.exe or its source repository.'}
 if(!$NssmPath){$NssmPath=$serviceExecutable}
 $application=Invoke-Nssm @('get',$serviceName,'Application')
 if($application -ine $binary){throw 'Existing TimeKeeper service belongs to a different executable; refusing takeover.'}
 $legacyNssmSettings=Get-NssmSettings
 $legacyNssmSettings | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $backup 'nssm-service.json')
 $legacyWasRunning=$existingService.State -eq 'Running'
}elseif($existingBackend -eq 'obey'){
 if($installManager -ne 'Bend2Will obey'){throw 'Existing Bend2Will service requires its obey.exe or Bend2Will source to be available.'}
 if($serviceExecutable -ine $obeyBinary){throw 'Existing Bend2Will service uses an unexpected obey.exe path; refusing takeover.'}
 if($existingService.PathName.IndexOf($binary,[StringComparison]::OrdinalIgnoreCase) -lt 0){throw 'Existing Bend2Will service belongs to a different TimeKeeper binary; refusing takeover.'}
 if($existingService.StartName -ine 'NT AUTHORITY\LocalService'){throw 'Existing Bend2Will service is not running as LocalService; refusing takeover.'}
 $metadataPath=Join-Path $state 'service/last-install.json'
 if(!(Test-Path -LiteralPath $metadataPath)){throw 'Existing Bend2Will service has no TimeKeeper install record; refusing takeover.'}
 $previousObeyMetadata=Get-Content -LiteralPath $metadataPath -Raw | ConvertFrom-Json
 if($previousObeyMetadata.ServiceManager -ne 'Bend2Will'){throw 'Existing service install record does not identify Bend2Will; refusing takeover.'}
 $legacyWasRunning=$existingService.State -eq 'Running'
 $serviceAlreadyConfigured=($previousObeyMetadata.Parameters -eq $parameters -and $previousObeyMetadata.LogPath -eq $serviceLog -and [long]$previousObeyMetadata.LogMaxBytes -eq 10485760)
}elseif($existingBackend -eq 'unknown'){
 throw "Refusing to replace unknown service image: $($existingService.PathName)"
}
 # Verify every listener's identity before stopping anything.
 foreach($owner in @(Get-TkListener)){$process=Get-CimInstance Win32_Process -Filter "ProcessId=$owner";if($process.ExecutablePath -ine $binary){throw 'Port 1618 belongs to another application.'}}
 if($legacy -and $wasEnabled){Disable-ScheduledTask -TaskName $legacyName -TaskPath '\' | Out-Null;$legacyDisabled=$true}
 if($existingService -and $existingService.State -ne 'Stopped'){
 if($existingBackend -eq 'nssm'){Invoke-Nssm @('stop',$serviceName)|Out-Null}
 elseif($existingBackend -eq 'obey'){Invoke-Obey $serviceExecutable @('stop',$serviceName)|Out-Null}
 (Get-Service $serviceName).WaitForStatus('Stopped',[TimeSpan]::FromSeconds(30))
}
 foreach($owner in @(Get-TkListener)){Stop-Process -Id $owner;$stoppedStandalone=$true;Wait-Process -Id $owner -Timeout 10 -ErrorAction SilentlyContinue}
 New-Item -ItemType Directory -Path (Join-Path $state 'app/bin'),(Join-Path $state 'log'),(Join-Path $state 'web') -Force | Out-Null
if($installManager -eq 'Bend2Will obey' -and $obeySource -ine $obeyBinary){
 if(Test-Path -LiteralPath $obeyBinary){Copy-Item -LiteralPath $obeyBinary -Destination (Join-Path $backup 'obey.previous.exe');$previousObeySaved=$true}
 Copy-Item -LiteralPath $obeySource -Destination $obeyBinary -Force
}
 if($Build){
  foreach($name in @('timekeeper','tk')){
   $dest=Join-Path $state "app/bin/$name.exe"
   if(Test-Path -LiteralPath $dest){Copy-Item -LiteralPath $dest -Destination (Join-Path $backup "$name.previous.exe");if($name -eq 'timekeeper'){$previousBinarySaved=$true}else{$previousTkSaved=$true}}
   Copy-Item -LiteralPath (Join-Path $backup "$name.exe") -Destination $dest -Force
  }
  $webState=Join-Path $state 'web';$webBackup=Join-Path $backup 'web.previous'
  if(Test-Path -LiteralPath $webState){Copy-Item -LiteralPath $webState -Destination $webBackup -Recurse;$previousWebSaved=$true}
  Copy-Item -Path (Join-Path $root 'web/*') -Destination $webState -Recurse -Force
 }
 if(!(Test-Path -LiteralPath (Join-Path $state 'web/index.html'))){throw 'Dashboard assets missing.'}
 # Dedicated low-privilege service account; no user password is collected.
 & icacls.exe $state /grant '*S-1-5-19:(OI)(CI)M' /T /Q | Out-Null;if($LASTEXITCODE -ne 0){throw 'Could not grant LocalService access to TimeKeeper runtime state.'}
 if($installManager -eq 'NSSM legacy fallback'){
  if(!$NssmPath){throw 'NSSM is required for the legacy fallback; set TIMEKEEPER_NSSM.'}
  if(!$existingService){Invoke-Nssm @('install',$serviceName,$binary)|Out-Null;$nssmInstalled=$true}
  $settings=[ordered]@{Application=$binary;AppDirectory=$root;AppParameters=$nssmParameters;DisplayName='TimeKeeper';Description='TimeKeeper project execution service';ObjectName='NT AUTHORITY\LocalService';Start='SERVICE_DELAYED_AUTO_START';AppStdout=$serviceLog;AppStderr=$serviceErrorLog;AppStdoutCreationDisposition='4';AppStderrCreationDisposition='4';AppRotateFiles='1';AppRotateBytes='10485760';AppRestartDelay='5000';AppThrottle='30000'}
  foreach($key in $settings.Keys){Invoke-Nssm @('set',$serviceName,$key,$settings[$key])|Out-Null}
  Invoke-Nssm @('set',$serviceName,'AppExit','Default','Restart')|Out-Null
 }else{
  if($existingBackend -eq 'nssm'){
   Invoke-Nssm @('remove',$serviceName,'confirm')|Out-Null
   $legacyNssmRemoved=$true
   Wait-ServiceRemoved
  }
  elseif($existingBackend -eq 'obey' -and !$serviceAlreadyConfigured){
   Invoke-Obey $serviceExecutable @('release',$serviceName)|Out-Null
   $obeyServiceRemoved=$true
   Wait-ServiceRemoved
  }
 }
 if(!$existingService -or $UpdateHosts){& $cliBinary hosts add;if($LASTEXITCODE -ne 0){throw 'Friendly hostname setup failed.'}}
 if($installManager -eq 'NSSM legacy fallback'){Invoke-NssmStartOrRestart 'start'}
 elseif($serviceAlreadyConfigured){Invoke-Obey $obeyBinary @('start',$serviceName)|Out-Null}
 else{$newObeyAttempted=$true;Install-ObeyService $parameters $serviceLog 10485760;$obeyInstalled=$true}
 Assert-ServiceHealth
 if($legacy -and $RetireLegacyLauncher){Unregister-ScheduledTask -TaskName $legacyName -TaskPath '\' -Confirm:$false;if(Get-ScheduledTask -TaskName $legacyName -TaskPath '\' -ErrorAction SilentlyContinue){throw 'Legacy task removal did not persist.'}}
elseif($legacy){Write-Output 'Legacy launcher task was retained; it remains disabled if it was enabled before migration.'}
 if($installManager -eq 'NSSM legacy fallback'){
 [pscustomobject]@{ServiceManager='NSSM';Service=$serviceName;Status='Running';Health='ok';Parameters=$nssmParameters;LogPath=$serviceLog;LogMaxBytes=10485760;Account='NT AUTHORITY\LocalService';Start='delayed-auto';LegacyTaskRemoved=[bool]($legacy -and $RetireLegacyLauncher);LegacyTaskRetained=[bool]($legacy -and !$RetireLegacyLauncher);Backup=$backup}|ConvertTo-Json -Depth 4 | Set-Content -LiteralPath $installRecordPath
}else{
 [pscustomobject]@{ServiceManager='Bend2Will';Service=$serviceName;Status='Running';Health='ok';ObeyPath=$obeyBinary;Parameters=$parameters;LogPath=$serviceLog;LogMaxBytes=10485760;Account='NT AUTHORITY\LocalService';Start='delayed-auto';RestartServiceOnFailure=$true;ServiceRestartDelay='5s';LegacyTaskRemoved=[bool]($legacy -and $RetireLegacyLauncher);LegacyTaskRetained=[bool]($legacy -and !$RetireLegacyLauncher);Backup=$backup}|ConvertTo-Json -Depth 4 | Set-Content -LiteralPath $installRecordPath
}
 Write-Output "TimeKeeper service verified; legacy task retired. Backup: $backup"
} catch {
 $failure=$_
 if($newObeyAttempted -or $obeyInstalled){
  try{$current=Get-CimInstance Win32_Service -Filter "Name='$serviceName'" -ErrorAction SilentlyContinue;if((Get-ServiceBackend $current) -eq 'obey'){Invoke-Obey (Get-ServiceExecutablePath $current.PathName) @('release',$serviceName)|Out-Null;Wait-ServiceRemoved}}catch{Write-Warning "Could not remove the failed Bend2Will service: $_"}
 }elseif($existingBackend -eq 'obey' -and $serviceAlreadyConfigured){
  try{Invoke-Obey $obeyBinary @('stop',$serviceName)|Out-Null}catch{}
 }
 if($nssmInstalled){try{Invoke-Nssm @('stop',$serviceName)|Out-Null;Invoke-Nssm @('remove',$serviceName,'confirm')|Out-Null;Wait-ServiceRemoved}catch{Write-Warning "Could not remove the failed NSSM service: $_"}}
 if($previousBinarySaved -and (Test-Path -LiteralPath (Join-Path $backup 'timekeeper.previous.exe'))){Copy-Item -LiteralPath (Join-Path $backup 'timekeeper.previous.exe') -Destination $binary -Force}
 if($previousTkSaved -and (Test-Path -LiteralPath (Join-Path $backup 'tk.previous.exe'))){Copy-Item -LiteralPath (Join-Path $backup 'tk.previous.exe') -Destination $cliBinary -Force}
 if($previousWebSaved -and (Test-Path -LiteralPath (Join-Path $backup 'web.previous'))){Copy-Item -Path (Join-Path $backup 'web.previous/*') -Destination (Join-Path $state 'web') -Recurse -Force}
 if($previousObeySaved -and (Test-Path -LiteralPath (Join-Path $backup 'obey.previous.exe'))){Copy-Item -LiteralPath (Join-Path $backup 'obey.previous.exe') -Destination $obeyBinary -Force}
 if($legacyNssmRemoved -and $legacyNssmSettings){
  try{Restore-NssmSettings $legacyNssmSettings;if($legacyWasRunning){Invoke-NssmStartOrRestart 'start';Assert-ServiceHealth}}catch{Write-Warning "Could not restore the previous NSSM service: $_"}
 }elseif($obeyServiceRemoved -and $previousObeyMetadata){
  try{Install-ObeyService $previousObeyMetadata.Parameters $previousObeyMetadata.LogPath ([long]$previousObeyMetadata.LogMaxBytes);if($legacyWasRunning){Invoke-Obey $obeyBinary @('start',$serviceName)|Out-Null;Assert-ServiceHealth}}catch{Write-Warning "Could not restore the previous Bend2Will service: $_"}
 }elseif($existingBackend -eq 'obey' -and $serviceAlreadyConfigured -and $legacyWasRunning){
  try{Invoke-Obey $obeyBinary @('start',$serviceName)|Out-Null;Assert-ServiceHealth}catch{Write-Warning "Could not restart the previous Bend2Will service: $_"}
 }elseif($existingBackend -eq 'nssm' -and !$legacyNssmRemoved -and $legacyNssmSettings){
  try{$svc=Get-Service -Name $serviceName -ErrorAction SilentlyContinue;if($svc -and $svc.Status -ne 'Stopped'){Invoke-Nssm @('stop',$serviceName)|Out-Null};Apply-NssmSettings $legacyNssmSettings;if($legacyWasRunning){Invoke-NssmStartOrRestart 'start';Assert-ServiceHealth}}catch{Write-Warning "Could not restore the previous NSSM service: $_"}
 }
 if($previousInstallRecordSaved -and (Test-Path -LiteralPath $previousInstallRecordPath)){Copy-Item -LiteralPath $previousInstallRecordPath -Destination $installRecordPath -Force}
 if($legacyDisabled -and (Get-ScheduledTask -TaskName $legacyName -TaskPath '\' -ErrorAction SilentlyContinue)){Enable-ScheduledTask -TaskName $legacyName -TaskPath '\' | Out-Null}
 Write-Error $failure
 exit 1
} finally {Stop-Transcript | Out-Null}
