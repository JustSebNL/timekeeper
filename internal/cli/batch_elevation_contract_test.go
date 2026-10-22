package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readBatchContractFile(t *testing.T, relativePath string) string {
	t.Helper()
	root, err := repoRootForTest()
	if err != nil {
		t.Fatalf("find repository root: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, relativePath))
	if err != nil {
		t.Fatalf("read %s: %v", relativePath, err)
	}
	return string(data)
}

func TestKickServerRequestsElevationOnlyOnUnhealthyServicePath(t *testing.T) {
	script := readBatchContractFile(t, filepath.Join("scripts", "service", "kick-server.bat"))
	healthyExit := strings.Index(script, "if not errorlevel 1 exit /b 0")
	if healthyExit < 0 {
		t.Fatal("healthy service path must return before invoking any restart action")
	}
	serviceRestart := strings.Index(script, "\"%TK%\" service restart")
	if serviceRestart < 0 {
		t.Fatal("unhealthy service path must invoke tk service restart, which routes through the UAC-aware service manager")
	}
	if serviceRestart < healthyExit {
		t.Fatal("service restart must occur only after the health gate fails")
	}
	if strings.Contains(script, "\"%NSSM%\" restart") {
			t.Fatal("kick-server must not bypass the supported elevated service manager with direct NSSM control")
		}
}

func TestLegacyServiceInstallBatchRequestsElevationBeforeMutation(t *testing.T) {
	script := readBatchContractFile(t, filepath.Join(".timekeeper", "scripts", "service", "service-install.bat"))
	batchAdminCheck := strings.Index(script, "WindowsPrincipal")
	if batchAdminCheck < 0 {
		t.Fatal("legacy service installer must detect its current elevation state")
	}
	helper := readBatchContractFile(t, filepath.Join("scripts", "service", "elevate-batch.ps1"))
	adminCheck := strings.Index(helper, "WindowsPrincipal")
	if adminCheck < 0 {
		t.Fatal("legacy service installer must check whether it is already elevated")
	}
	runas := strings.Index(helper, "RunAs")
	if runas < 0 {
		t.Fatal("legacy service installer must request UAC elevation when not elevated")
	}
	mutation := strings.Index(script, "\"%NSSM%\" install")
	if mutation < 0 {
		t.Fatal("expected existing NSSM installation operation")
	}
	elevateCall := strings.Index(script, "elevate-batch.ps1")
	if elevateCall < 0 {
		t.Fatal("legacy service installer must invoke the shared batch elevation helper")
	}
	if batchAdminCheck > elevateCall || elevateCall > mutation {
		t.Fatal("elevation check and UAC relaunch must precede service mutation")
	}
}
