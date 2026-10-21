[CmdletBinding()]
param(
  [Parameter(Mandatory = $true)]
  [ValidateScript({ Test-Path -LiteralPath $_ -PathType Leaf })]
  [string]$BatchPath,
  [Parameter(Mandatory = $true)]
  [ValidateScript({ Test-Path -LiteralPath $_ -PathType Container })]
  [string]$WorkingDirectory
)

$ErrorActionPreference = 'Stop'
$BatchPath = (Resolve-Path -LiteralPath $BatchPath).Path
$WorkingDirectory = (Resolve-Path -LiteralPath $WorkingDirectory).Path
$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = New-Object Security.Principal.WindowsPrincipal($identity)
$isAdministrator = $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
$arguments = @('/d', '/s', '/c', ('call "' + $BatchPath + '"'))
$start = @{
  FilePath = $env:ComSpec
  ArgumentList = $arguments
  WorkingDirectory = $WorkingDirectory
  PassThru = $true
  Wait = $true
  ErrorAction = 'Stop'
}
if (!$isAdministrator) {
  $start.Verb = 'RunAs'
}

try {
  $child = Start-Process @start
  exit $child.ExitCode
} catch {
  [Console]::Error.WriteLine(('Could not launch the batch file: {0}' -f $_.Exception.Message))
  exit 1
}
