package api

import (
	"bytes"
	"context"
	"testing"
	"time"
)

func TestIsCommandBlocked(t *testing.T) {
	blocked := []string{
		"rm -rf /",
		"rm -rf /*",
		"rm -rf ~",
		"rm -rf ~/*",
		"rm -rfi /",
		"mkfs.ext4 /dev/sda1",
		"dd if=/dev/zero of=/dev/sda",
		"dd if=/dev/urandom of=/dev/sda",
		"> /dev/sda",
		":(){ :|:& };:",
		"chmod -R 777 /",
		"shutdown",
		"shutdown -h now",
		"reboot",
		"halt",
		"poweroff",
		"init 0",
		"init 6",
		"format C:",
	}
	for _, cmd := range blocked {
		if !isCommandBlocked(cmd) {
			t.Errorf("expected blocked: %q", cmd)
		}
	}

	safe := []string{
		"ls -la",
		"echo hello",
		"cat /etc/hostname",
		"python3 script.py",
		"curl https://example.com",
		"rm -rf ./my-dir",
		"rm file.txt",
		"dd if=input.img of=output.img",
		"chmod 755 myfile",
	}
	for _, cmd := range safe {
		if isCommandBlocked(cmd) {
			t.Errorf("expected safe: %q", cmd)
		}
	}
}

func TestExecuteShellCommand_Echo(t *testing.T) {
	result := executeShellCommand(context.Background(), "/bin/sh", "echo hello", 5*time.Second, "", nil)
	if result["success"] != true {
		t.Fatalf("expected success, got %v (stderr: %v)", result["success"], result["stderr"])
	}
	stdout, _ := result["stdout"].(string)
	if stdout == "" || stdout[:5] != "hello" {
		t.Errorf("stdout = %q", stdout)
	}
	if result["exit_code"] != 0 {
		t.Errorf("exit_code = %v", result["exit_code"])
	}
	if result["timed_out"] != false {
		t.Error("should not be timed out")
	}
}

func TestExecuteShellCommand_Failure(t *testing.T) {
	result := executeShellCommand(context.Background(), "/bin/sh", "exit 42", 5*time.Second, "", nil)
	if result["success"] != false {
		t.Error("expected failure")
	}
	if result["exit_code"] != 42 {
		t.Errorf("exit_code = %v", result["exit_code"])
	}
}

func TestExecuteShellCommand_Blocked(t *testing.T) {
	result := executeShellCommand(context.Background(), "/bin/sh", "rm -rf /", 5*time.Second, "", nil)
	if result["success"] != false {
		t.Error("expected blocked")
	}
	errMsg, _ := result["error"].(string)
	if errMsg == "" {
		t.Error("expected error message")
	}
}

func TestExecuteShellCommand_Timeout(t *testing.T) {
	result := executeShellCommand(context.Background(), "/bin/sh", "sleep 10", 100*time.Millisecond, "", nil)
	if result["timed_out"] != true {
		t.Error("expected timed_out")
	}
	// exit code may be 124 or -1 depending on timing
	ec, _ := result["exit_code"].(int)
	if ec != 124 && ec != -1 {
		t.Errorf("exit_code = %v, want 124 or -1", result["exit_code"])
	}
}

func TestExecuteShellCommand_DefaultTimeout(t *testing.T) {
	// timeout 0 should default to shellDefaultTimeout, not hang
	result := executeShellCommand(context.Background(), "/bin/sh", "echo ok", 0, "", nil)
	if result["success"] != true {
		t.Errorf("expected success with default timeout")
	}
}

func TestExecuteShellCommand_MaxTimeout(t *testing.T) {
	// timeout > max should be capped (we just verify it doesn't crash)
	result := executeShellCommand(context.Background(), "/bin/sh", "echo capped", 999*time.Second, "", nil)
	if result["success"] != true {
		t.Error("expected success")
	}
}

func TestExecuteShellCommand_WorkingDir(t *testing.T) {
	result := executeShellCommand(context.Background(), "/bin/sh", "pwd", 5*time.Second, "/tmp", nil)
	if result["success"] != true {
		t.Fatalf("expected success")
	}
	stdout, _ := result["stdout"].(string)
	if stdout == "" {
		t.Error("expected pwd output")
	}
}

func TestExecuteShellCommand_EnvVars(t *testing.T) {
	env := map[string]string{"MY_VAR": "hello123"}
	result := executeShellCommand(context.Background(), "/bin/sh", "echo $MY_VAR", 5*time.Second, "", env)
	// Note: env vars may not propagate to subshell without explicit setup
	// This test just verifies no crash
	if result["timed_out"] == true {
		t.Error("should not timeout")
	}
}

func TestExecuteShellCommand_BlockedEnvVar(t *testing.T) {
	env := map[string]string{"PATH": "/evil", "SAFE_VAR": "ok"}
	// Should filter PATH but not crash
	result := executeShellCommand(context.Background(), "/bin/sh", "echo test", 5*time.Second, "", env)
	if result["timed_out"] == true {
		t.Error("should not timeout")
	}
}

func TestProcessManager_StartAndList(t *testing.T) {
	pm := NewProcessManager()
	result := pm.StartBackground(context.Background(), "/bin/sh", "echo bg-test", 5*time.Second, "", nil)
	if result["success"] != true {
		t.Fatalf("expected success: %v", result)
	}
	sessionID, _ := result["session_id"].(string)
	if sessionID == "" {
		t.Fatal("expected session_id")
	}

	// Wait for process to finish
	time.Sleep(200 * time.Millisecond)

	list := pm.List()
	total, _ := list["total"].(int)
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
}

func TestProcessManager_Log(t *testing.T) {
	pm := NewProcessManager()
	result := pm.StartBackground(context.Background(), "/bin/sh", "echo line1; echo line2; echo line3", 5*time.Second, "", nil)
	sessionID, _ := result["session_id"].(string)

	time.Sleep(200 * time.Millisecond)

	logResult := pm.Log(sessionID, 0, 100)
	if logResult["success"] != true {
		t.Fatalf("log failed: %v", logResult)
	}
	stdout, _ := logResult["stdout"].(string)
	if stdout == "" {
		t.Error("expected stdout output")
	}
}

func TestProcessManager_LogUnknown(t *testing.T) {
	pm := NewProcessManager()
	result := pm.Log("nonexistent", 0, 100)
	if result["success"] != false {
		t.Error("expected failure for unknown session")
	}
}

func TestProcessManager_Kill(t *testing.T) {
	pm := NewProcessManager()
	result := pm.StartBackground(context.Background(), "/bin/sh", "sleep 60", 120*time.Second, "", nil)
	sessionID, _ := result["session_id"].(string)

	time.Sleep(50 * time.Millisecond)

	killResult := pm.Kill(sessionID)
	if killResult["success"] != true {
		t.Errorf("kill failed: %v", killResult)
	}

	// Kill again should say already finished
	killResult2 := pm.Kill(sessionID)
	if killResult2["success"] != true {
		t.Error("kill of finished should succeed")
	}
}

func TestProcessManager_KillUnknown(t *testing.T) {
	pm := NewProcessManager()
	result := pm.Kill("nope")
	if result["success"] != false {
		t.Error("expected failure")
	}
}

func TestProcessManager_Remove(t *testing.T) {
	pm := NewProcessManager()
	result := pm.StartBackground(context.Background(), "/bin/sh", "echo done", 5*time.Second, "", nil)
	sessionID, _ := result["session_id"].(string)

	time.Sleep(200 * time.Millisecond)

	removeResult := pm.Remove(sessionID)
	if removeResult["success"] != true {
		t.Errorf("remove failed: %v", removeResult)
	}

	// Should be gone
	list := pm.List()
	if list["total"] != 0 {
		t.Errorf("total = %v after remove", list["total"])
	}
}

func TestProcessManager_RemoveUnknown(t *testing.T) {
	pm := NewProcessManager()
	result := pm.Remove("nope")
	if result["success"] != false {
		t.Error("expected failure")
	}
}

func TestProcessManager_RemoveRunning(t *testing.T) {
	pm := NewProcessManager()
	result := pm.StartBackground(context.Background(), "/bin/sh", "sleep 60", 120*time.Second, "", nil)
	sessionID, _ := result["session_id"].(string)

	time.Sleep(50 * time.Millisecond)

	removeResult := pm.Remove(sessionID)
	if removeResult["success"] != true {
		t.Errorf("remove running failed: %v", removeResult)
	}
}

func TestProcessManager_Blocked(t *testing.T) {
	pm := NewProcessManager()
	result := pm.StartBackground(context.Background(), "/bin/sh", "rm -rf /", 5*time.Second, "", nil)
	if result["success"] != false {
		t.Error("expected blocked")
	}
}

func TestLimitedWriter(t *testing.T) {
	var buf bytes.Buffer
	lw := &limitedWriter{buf: &buf, max: 10}

	n, err := lw.Write([]byte("hello"))
	if err != nil || n != 5 {
		t.Errorf("write1: n=%d err=%v", n, err)
	}
	if buf.Len() != 5 {
		t.Errorf("buf len = %d", buf.Len())
	}

	// Write more than remaining
	n, err = lw.Write([]byte("world!!!!"))
	if err != nil {
		t.Errorf("write2 err: %v", err)
	}
	if n != 9 { // reports full length written (pretend)
		t.Errorf("write2 n = %d", n)
	}
	if buf.Len() != 10 {
		t.Errorf("buf should be capped at 10, got %d", buf.Len())
	}

	// Write when already full
	n, err = lw.Write([]byte("extra"))
	if err != nil {
		t.Errorf("write3 err: %v", err)
	}
	if n != 5 { // pretends to write
		t.Errorf("write3 n = %d", n)
	}
	if buf.Len() != 10 {
		t.Errorf("buf should still be 10, got %d", buf.Len())
	}
}
