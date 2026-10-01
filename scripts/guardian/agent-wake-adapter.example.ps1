# Runtime-neutral Windows adapter. AGENT_RUNTIME_SUBMIT must be a trusted
# supervisor executable implementing the blocking submit contract below.
param([string]$Reminder = 'Continue the already-authorized TimeKeeper work.')
$ErrorActionPreference = 'Stop'
if ($env:AGENT_ID -notmatch '^[A-Za-z0-9_.-]{1,128}$' -or $env:AGENT_ID -in @('.', '..')) {
    throw 'A stable AGENT_ID is required.'
}
if ($env:AGENT_LAST_NUDGE_ID -notmatch '^[1-9][0-9]*$') { throw 'A pending nudge ID is required.' }
if (!$env:AGENT_RUNTIME_SUBMIT) { throw 'Set AGENT_RUNTIME_SUBMIT to a trusted supervisor executable.' }
$stateDir = $env:TIMEKEEPER_STATE_DIR
if (!$stateDir) { $stateDir = Join-Path (Get-Location) '.timekeeper/state' }
[System.IO.Directory]::CreateDirectory($stateDir) | Out-Null
$lockPath = Join-Path $stateDir ('adapter-' + $env:AGENT_ID + '.lock')
$lease = $null
try {
    try {
        # The OS releases this exclusive handle on process exit, including a
        # crash. Keep the file: deleting it would race the next lock owner.
        $lease = [System.IO.File]::Open($lockPath, 'OpenOrCreate', 'ReadWrite', 'None')
    } catch [System.IO.IOException] {
        if (($_.Exception.HResult -band 0xffff) -in @(32, 33)) { exit 75 }
        throw
    }
    # No Invoke-Expression. Reminder is one data argument. The supervisor must
    # deduplicate agent + nudge + continuation cycle, wait for verified work or
    # a durable blocker, and return failure when that boundary is unknown.
    & $env:AGENT_RUNTIME_SUBMIT --agent $env:AGENT_ID --nudge $env:AGENT_LAST_NUDGE_ID --reason $Reminder
    if ($null -eq $LASTEXITCODE) { throw 'Supervisor did not return a process exit code.' }
    exit $LASTEXITCODE
} finally {
    if ($lease) { $lease.Dispose() }
}
