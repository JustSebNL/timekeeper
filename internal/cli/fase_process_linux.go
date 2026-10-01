//go:build linux

package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

func processSessionSource() (string, error) {
	boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return "", err
	}
	pid := os.Getppid()
	for depth := 0; depth < 8; depth++ {
		data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
		if err != nil {
			return "", err
		}
		raw := string(data)
		begin, end := strings.Index(raw, "("), strings.LastIndex(raw, ")")
		if begin < 0 || end < begin {
			return "", fmt.Errorf("invalid calling process metadata")
		}
		fields := strings.Fields(raw[end+1:])
		if len(fields) < 20 {
			return "", fmt.Errorf("incomplete calling process metadata")
		}
		name := raw[begin+1 : end]
		parent, err := strconv.Atoi(fields[1])
		if err != nil {
			return "", err
		}
		shell := name == "sh" || name == "bash" || name == "dash" || name == "zsh" || name == "fish" || name == "pwsh"
		if !shell || parent <= 1 || parent == pid || depth == 7 {
			return fmt.Sprintf("linux/%s/%d/%s", strings.TrimSpace(string(boot)), pid, fields[19]), nil
		}
		pid = parent
	}
	return "", fmt.Errorf("calling process identity unavailable")
}
