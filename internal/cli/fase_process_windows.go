//go:build windows

package cli

import (
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"strings"
	"unsafe"
)

func processSessionSource() (string, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(snapshot)
	entries := make(map[uint32]windows.ProcessEntry32)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		entries[entry.ProcessID] = entry
	}
	pid := uint32(os.Getppid())
	for depth := 0; depth < 8; depth++ {
		p, ok := entries[pid]
		if !ok {
			break
		}
		name := strings.ToLower(windows.UTF16ToString(p.ExeFile[:]))
		if name != "pwsh.exe" && name != "powershell.exe" && name != "cmd.exe" && name != "bash.exe" && name != "sh.exe" {
			break
		}
		parent, ok := entries[p.ParentProcessID]
		if !ok || p.ParentProcessID == 0 || p.ParentProcessID == pid {
			break
		}
		parentName := strings.ToLower(windows.UTF16ToString(parent.ExeFile[:]))
		if parentName == "explorer.exe" || parentName == "services.exe" || parentName == "wininit.exe" {
			break
		}
		pid = p.ParentProcessID
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", fmt.Errorf("read calling host identity: %w", err)
	}
	defer windows.CloseHandle(handle)
	var created, exited, kernel, user windows.Filetime
	if err = windows.GetProcessTimes(handle, &created, &exited, &kernel, &user); err != nil {
		return "", err
	}
	host, err := os.Hostname()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("windows/%s/%d/%d/%d", host, pid, created.HighDateTime, created.LowDateTime), nil
}
