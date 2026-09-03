#!/usr/bin/env bash
# Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.
# Sourced by scripts/service/service-manager.sh and the install wrappers.
# Provides:
#   tk_probe_loopback_port <host> <port> -> "free" | "in-use" | "unknown"
#   tk_resolve_proxy_addr [default]      -> echoes the resolved address
#                                           (or empty string if disabled)
#
# The probe uses bash's built-in /dev/tcp, which is available in bash
# 2.04+ on every platform we target (Linux, WSL, Git Bash, macOS).
# We deliberately do NOT depend on nc, ss, or python for portability.

# tk_probe_loopback_port <host> <port>
#   Prints one of: "free", "in-use", "unknown".
#   The probe checks whether the port is in LISTEN state using ss
#   (preferred) or netstat as a fallback. We deliberately do NOT
#   open a TCP connection here: opening a connection to a listening
#   server has been observed to hang the bash subshell even with
#   fd cleanup. ss/netstat is a passive, kernel-state read, with
#   no hang surface and no socket state to release.
#
#   CAVEAT: on WSL2 the Linux ss/netstat does NOT see Windows-side
#   listeners. The probe is therefore reliable on native Linux and
#   macOS, and a no-op on WSL. On WSL we return "unknown" so the
#   caller knows the install-time check cannot help and the user
#   should rely on the timekeeper.exe runtime bind behavior (which
#   IS authoritative, including on WSL).
tk_probe_loopback_port() {
  local host="$1"
  local port="$2"
  if [[ -z "$host" || -z "$port" ]]; then
    echo "unknown"
    return 0
  fi
  # WSL2: skip the probe. The WSL netstat does not see Windows-side
  # listeners, and the install will not gain anything from a false
  # negative. The runtime bind inside timekeeper.exe is authoritative.
  if [[ -f /proc/version ]] && grep -qiE 'microsoft|wsl' /proc/version 2>/dev/null; then
    echo "unknown"
    return 0
  fi
  if command -v ss >/dev/null 2>&1; then
    if ss -Hltn 2>/dev/null | awk '{print $4}' | grep -qE "[:.]${port}\$"; then
      echo "in-use"
      return 0
    fi
    echo "free"
    return 0
  fi
  if command -v netstat >/dev/null 2>&1; then
    if netstat -ltn 2>/dev/null | awk '{print $4}' | grep -qE "[:.]${port}\$"; then
      echo "in-use"
      return 0
    fi
    echo "free"
    return 0
  fi
  echo "unknown"
  return 0
}

# tk_resolve_proxy_addr [default]
#   Decides what -proxy-addr should be passed to the timekeeper
#   binary, in priority order:
#     1. $TIMEKEEPER_PROXY_ADDR (env override, wins unconditionally)
#     2. persisted in INSTALLATION.env
#     3. probe 127.0.0.1:<default>; if free, use it
#     4. prompt the user for one of the documented three options
#        and persist the choice to INSTALLATION.env
#     5. if the prompt is unavailable (non-interactive shell), warn
#        and fall back to the default; the user can override via
#        TIMEKEEPER_PROXY_ADDR on the next install attempt.
#   An empty default argument means "no proxy" (the user already
#   chose the disabled path on a previous run, persisted in
#   INSTALLATION.env).
tk_resolve_proxy_addr() {
  local default="${1:-127.0.0.1:80}"
  local installation_env="${TIMEKEEPER_INSTALLATION_ENV:-$REPO/.timekeeper/app/INSTALLATION.env}"

  # 1. explicit env wins
  if [[ -n "${TIMEKEEPER_PROXY_ADDR:-}" ]]; then
    echo "$TIMEKEEPER_PROXY_ADDR"
    return 0
  fi

  # 2. persisted in INSTALLATION.env
  if [[ -f "$installation_env" ]]; then
    local persisted
    persisted="$(grep -E '^TIMEKEEPER_PROXY_ADDR=' "$installation_env" 2>/dev/null | cut -d= -f2- || true)"
    if [[ -n "$persisted" ]]; then
      echo "$persisted"
      return 0
    fi
  fi

  # If the user previously chose "no proxy" we keep that choice
  # even if a default is given — the sentinel is TIMEKEEPER_PROXY_DISABLED=1.
  if [[ -f "$installation_env" ]] && grep -qE '^TIMEKEEPER_PROXY_DISABLED=1' "$installation_env" 2>/dev/null; then
    echo ""
    return 0
  fi

  # 3. probe the default; if free, use it
  local host="${default%%:*}"
  local port="${default##*:}"
  local status
  status="$(tk_probe_loopback_port "$host" "$port")"
  if [[ "$status" == "free" ]]; then
    echo "$default"
    return 0
  fi
  if [[ "$status" != "in-use" ]]; then
    # unknown — fall back to default; warn
    log "[service] WARN: could not probe $default; assuming free. If the timekeeper proxy fails to bind, set TIMEKEEPER_PROXY_ADDR."
    echo "$default"
    return 0
  fi

  # 4. prompt the user
  if ! tk_prompt_proxy_choice "$default" "$installation_env"; then
    log "[service] WARN: could not prompt for proxy address; defaulting to $default. If binding fails, restart with TIMEKEEPER_PROXY_ADDR set."
    echo "$default"
    return 0
  fi

  # Re-read whatever tk_prompt_proxy_choice persisted.
  if [[ -f "$installation_env" ]]; then
    if grep -qE '^TIMEKEEPER_PROXY_DISABLED=1' "$installation_env" 2>/dev/null; then
      echo ""
      return 0
    fi
    local persisted
    persisted="$(grep -E '^TIMEKEEPER_PROXY_ADDR=' "$installation_env" 2>/dev/null | cut -d= -f2- || true)"
    if [[ -n "$persisted" ]]; then
      echo "$persisted"
      return 0
    fi
  fi
  echo "$default"
  return 0
}

# tk_prompt_proxy_choice <default> <installation_env>
#   Interactive: explains the port-80 conflict and offers the three
#   documented options. Persists the choice to installation_env.
#   Returns 0 on success, 1 if the prompt could not be performed.
tk_prompt_proxy_choice() {
  local default="$1"
  local installation_env="$2"

  # Detect interactivity. If stdin is not a TTY, we cannot prompt.
  if [[ ! -t 0 ]]; then
    return 1
  fi

  log "[service] Port ${default##*:} on ${default%%:*} is already in use."
  log "[service] This is the proxy address for the friendly URLs (timekeeper.local)."
  log "[service] Choose one:"
  log "[service]   1) Free port ${default##*:} on this host and re-run the install"
  log "[service]   2) Use a different port (you will see timekeeper.local:<port> in the URL)"
  log "[service]   3) Run without the friendly-URL proxy (only 127.0.0.1:1618 will work)"
  local choice
  local default_choice="2"
  while true; do
    if ! read -r -t 30 -p "[service] Choose [1/2/3] (default $default_choice): " choice; then
      log "[service] No interactive input; defaulting to option $default_choice."
      choice="$default_choice"
    fi
    choice="${choice:-$default_choice}"
    case "$choice" in
      1)
        log "[service] Please free port ${default##*:} on this host (e.g. 'netstat -ano | findstr :${default##*:}' to find the holder), then re-run the install."
        return 1
        ;;
      2)
        local new_port=""
        while true; do
          if ! read -r -t 30 -p "[service] Enter a different port (e.g. 8080): " new_port; then
            new_port="8080"
          fi
          new_port="${new_port:-8080}"
          if [[ "$new_port" =~ ^[0-9]+$ ]] && (( new_port >= 1 && new_port <= 65535 )); then
            break
          fi
          log "[service] '$new_port' is not a valid port number. Try again."
        done
        local new_addr="${default%%:*}:$new_port"
        tk_persist_env "TIMEKEEPER_PROXY_ADDR" "$new_addr" "$installation_env"
        log "[service] Persisted TIMEKEEPER_PROXY_ADDR=$new_addr to $installation_env"
        log "[service] The friendly URL will be http://timekeeper.local:$new_port/"
        return 0
        ;;
      3)
        tk_persist_env "TIMEKEEPER_PROXY_ADDR" "" "$installation_env"
        tk_persist_env "TIMEKEEPER_PROXY_DISABLED" "1" "$installation_env"
        log "[service] Persisted TIMEKEEPER_PROXY_DISABLED=1; the proxy listener is disabled."
        return 0
        ;;
      *)
        log "[service] '$choice' is not a valid option. Enter 1, 2, or 3."
        ;;
    esac
  done
}

# tk_persist_env <key> <value> <installation_env>
#   Writes or replaces a KEY=VALUE line in INSTALLATION.env, preserving
#   the existing file. Creates the file if it does not exist.
tk_persist_env() {
  local key="$1"
  local value="$2"
  local file="$3"
  mkdir -p "$(dirname "$file")"
  if [[ -f "$file" ]]; then
    # Remove existing line with this key, then append.
    local tmp
    tmp="$(mktemp "${file}.XXXXXX")"
    grep -v -E "^${key}=" "$file" > "$tmp" || true
    printf '%s=%s\n' "$key" "$value" >> "$tmp"
    cat "$tmp" > "$file"
    rm -f "$tmp"
  else
    printf 'TIMEKEEPER_PERSISTED_AT=%s\n%s=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$key" "$value" > "$file"
  fi
  chmod 600 "$file" 2>/dev/null || true
}
