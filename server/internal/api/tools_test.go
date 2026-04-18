package api

import (
	"context"
	"testing"
)

func TestToolGetLocalTime(t *testing.T) {
	// toolGetLocalTime is a method on Server, but only uses args -- we can test with nil server fields
	s := &Server{}
	result := s.toolGetLocalTime(context.Background(), "", nil)
	if result["success"] != true {
		t.Fatalf("expected success: %v", result)
	}
	if result["iso_datetime"] == nil || result["iso_datetime"] == "" {
		t.Error("missing iso_datetime")
	}
	if result["day_of_week"] == nil || result["day_of_week"] == "" {
		t.Error("missing day_of_week")
	}
	if result["timezone"] == nil || result["timezone"] == "" {
		t.Error("missing timezone")
	}
}

func TestToolGetLocalTime_WithTimezone(t *testing.T) {
	s := &Server{}
	result := s.toolGetLocalTime(context.Background(), "", map[string]any{"timezone": "America/New_York"})
	if result["success"] != true {
		t.Fatalf("expected success: %v", result)
	}
	tz, _ := result["timezone"].(string)
	if tz != "America/New_York" {
		t.Errorf("timezone = %q, want America/New_York", tz)
	}
}

func TestToolGetLocalTime_BadTimezone(t *testing.T) {
	s := &Server{}
	result := s.toolGetLocalTime(context.Background(), "", map[string]any{"timezone": "Not/Real"})
	if result["success"] != true {
		 t.Fatal("should still succeed with fallback to local")
	}
}

func TestExtractTextContent(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"<b>hello</b>", "hello"},
		{"<a href='x'>link text</a>", "link text"},
		{"plain text", "plain text"},
		{"<div><p>nested</p></div>", "nested"},
		{"<br/>", ""},
		{"", ""},
		{"no tags at all", "no tags at all"},
		{"<script>alert('xss')</script>safe", "alert('xss')safe"},
	}
	for _, tt := range tests {
		got := extractTextContent(tt.input)
		if got != tt.want {
			t.Errorf("extractTextContent(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseDuckDuckGoLite_Empty(t *testing.T) {
	results := parseDuckDuckGoLite("")
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestParseDuckDuckGoLite_WithResults(t *testing.T) {
	html := `<html>
<tr>
  <td><a class="result-link" href="https://example.com">Example Title</a></td>
</tr>
<tr>
  <td class="result-snippet">This is the snippet text</td>
</tr>
</html>`
	results := parseDuckDuckGoLite(html)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0]["title"] != "Example Title" {
		t.Errorf("title = %q", results[0]["title"])
	}
	if results[0]["url"] != "https://example.com" {
		t.Errorf("url = %q", results[0]["url"])
	}
}

func TestParseDuckDuckGoLite_MaxResults(t *testing.T) {
	// Generate 15 result links, should cap at 10
	html := ""
	for i := 0; i < 15; i++ {
		html += `<a class="result-link" href="https://example.com/` + string(rune('a'+i)) + `">Title</a>` + "\n"
	}
	results := parseDuckDuckGoLite(html)
	if len(results) > 10 {
		t.Errorf("expected max 10 results, got %d", len(results))
	}
}

func TestAutoDetectEmailServer(t *testing.T) {
	tests := []struct {
		email    string
		wantIMAP string
		wantSMTP string
	}{
		{"user@gmail.com", "imap.gmail.com", "smtp.gmail.com"},
		{"user@googlemail.com", "imap.gmail.com", "smtp.gmail.com"},
		{"user@outlook.com", "outlook.office365.com", "smtp.office365.com"},
		{"user@hotmail.com", "outlook.office365.com", "smtp.office365.com"},
		{"user@yahoo.com", "imap.mail.yahoo.com", "smtp.mail.yahoo.com"},
		{"user@icloud.com", "imap.mail.me.com", "smtp.mail.me.com"},
		{"user@aol.com", "imap.aol.com", "smtp.aol.com"},
		{"user@protonmail.com", "127.0.0.1", "127.0.0.1"},
		{"user@custom.org", "imap.custom.org", "smtp.custom.org"},
	}
	for _, tt := range tests {
		result := autoDetectEmailServer(tt.email)
		if result == nil {
			t.Fatalf("autoDetectEmailServer(%q) = nil", tt.email)
		}
		if result.imapHost != tt.wantIMAP {
			t.Errorf("autoDetect(%q) imap = %q, want %q", tt.email, result.imapHost, tt.wantIMAP)
		}
		if result.smtpHost != tt.wantSMTP {
			t.Errorf("autoDetect(%q) smtp = %q, want %q", tt.email, result.smtpHost, tt.wantSMTP)
		}
	}
}

func TestAutoDetectEmailServer_Invalid(t *testing.T) {
	result := autoDetectEmailServer("not-an-email")
	if result != nil {
		t.Error("expected nil for invalid email")
	}
}

func TestToolManageProcess_UnknownAction(t *testing.T) {
	s := &Server{}
	result, err := s.toolManageProcess(map[string]any{"action": "dance"})
	if err != nil {
		t.Fatal(err)
	}
	if result["success"] != false {
		t.Error("expected failure for unknown action")
	}
}

func TestToolManageProcess_MissingAction(t *testing.T) {
	s := &Server{}
	result, err := s.toolManageProcess(map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if result["success"] != false {
		t.Error("expected failure for missing action")
	}
}

func TestToolManageProcess_List(t *testing.T) {
	s := &Server{}
	result, err := s.toolManageProcess(map[string]any{"action": "list"})
	if err != nil {
		t.Fatal(err)
	}
	// Should have running/finished keys
	if _, ok := result["running"]; !ok {
		t.Error("missing 'running' key")
	}
	if _, ok := result["finished"]; !ok {
		t.Error("missing 'finished' key")
	}
}

func TestToolManageProcess_LogMissingSessionID(t *testing.T) {
	s := &Server{}
	result, err := s.toolManageProcess(map[string]any{"action": "log"})
	if err != nil {
		t.Fatal(err)
	}
	if result["success"] != false {
		t.Error("expected failure without session_id")
	}
}

func TestToolManageProcess_KillMissingSessionID(t *testing.T) {
	s := &Server{}
	result, err := s.toolManageProcess(map[string]any{"action": "kill"})
	if err != nil {
		t.Fatal(err)
	}
	if result["success"] != false {
		t.Error("expected failure without session_id")
	}
}

func TestToolManageProcess_RemoveMissingSessionID(t *testing.T) {
	s := &Server{}
	result, err := s.toolManageProcess(map[string]any{"action": "remove"})
	if err != nil {
		t.Fatal(err)
	}
	if result["success"] != false {
		t.Error("expected failure without session_id")
	}
}

func TestToolExecuteShellCommand_Disabled(t *testing.T) {
	s := &Server{serverShellEnabled: false}
	result, err := s.toolExecuteShellCommand(context.TODO(), "user1", map[string]any{"command": "echo hi"})
	if err != nil {
		t.Fatal(err)
	}
	if result["success"] != false {
		t.Error("expected failure when shell disabled")
	}
}

func TestToolExecuteShellCommand_MissingCommand(t *testing.T) {
	s := &Server{serverShellEnabled: true}
	result, err := s.toolExecuteShellCommand(context.TODO(), "user1", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if result["success"] != false {
		t.Error("expected failure without command")
	}
}

func TestToolSendNotification_MissingFields(t *testing.T) {
	s := &Server{realtimeHub: nil}
	result, err := s.toolSendNotification(context.TODO(), "u1", map[string]any{"title": "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if result["success"] != false {
		t.Error("expected failure without body")
	}

	result, err = s.toolSendNotification(context.TODO(), "u1", map[string]any{"body": "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if result["success"] != false {
		t.Error("expected failure without title")
	}
}

func TestToolStoreMemory_MissingContent(t *testing.T) {
	s := &Server{}
	result, err := s.toolStoreMemory(context.TODO(), "u1", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if result["success"] != false {
		t.Error("expected failure without content")
	}
}

func TestToolForgetMemory_MissingID(t *testing.T) {
	s := &Server{}
	result, err := s.toolForgetMemory(context.TODO(), "u1", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if result["success"] != false {
		t.Error("expected failure without memoryId")
	}
}

func TestToolLearnMemory_MissingContent(t *testing.T) {
	s := &Server{}
	result, err := s.toolLearnMemory(context.TODO(), "u1", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if result["success"] != false {
		t.Error("expected failure without content")
	}
}

func TestToolReinforceMemory_MissingID(t *testing.T) {
	s := &Server{}
	result, err := s.toolReinforceMemory(context.TODO(), "u1", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if result["success"] != false {
		t.Error("expected failure without memoryId")
	}
}

func TestToolPromoteLearning_MissingID(t *testing.T) {
	s := &Server{}
	result, err := s.toolPromoteLearning(context.TODO(), "u1", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if result["success"] != false {
		t.Error("expected failure without memoryId")
	}
}

func TestToolScheduleTask_MissingFields(t *testing.T) {
	s := &Server{}
	result, err := s.toolScheduleTask(context.TODO(), "u1", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if result["success"] != false {
		t.Error("expected failure without prompt and executeAt")
	}
}

func TestToolScheduleTask_BadDate(t *testing.T) {
	s := &Server{}
	result, err := s.toolScheduleTask(context.TODO(), "u1", map[string]any{
		"prompt":    "test",
		"executeAt": "not-a-date",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result["success"] != false {
		t.Error("expected failure for bad date")
	}
}

func TestToolCancelTask_MissingID(t *testing.T) {
	s := &Server{}
	result, err := s.toolCancelTask(context.TODO(), "u1", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if result["success"] != false {
		t.Error("expected failure without taskId")
	}
}

func TestToolSetupEmail_MissingAddress(t *testing.T) {
	s := &Server{}
	result, err := s.toolSetupEmail(context.TODO(), "u1", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if result["success"] != false {
		t.Error("expected failure without address")
	}
}

func TestToolSetupEmail_MissingPassword(t *testing.T) {
	s := &Server{}
	result, err := s.toolSetupEmail(context.TODO(), "u1", map[string]any{"address": "test@gmail.com"})
	if err != nil {
		t.Fatal(err)
	}
	if result["success"] != false {
		t.Error("expected failure without password")
	}
}

func TestToolReadEmail_MissingID(t *testing.T) {
	s := &Server{}
	result, err := s.toolReadEmail(context.TODO(), "u1", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if result["success"] != false {
		t.Error("expected failure without messageId")
	}
}

func TestExecuteTool_UnknownTool(t *testing.T) {
	s := &Server{}
	result, err := s.executeTool(context.TODO(), "u1", "nonexistent_tool", nil)
	if err != nil {
		t.Fatal(err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", result)
	}
	if m["success"] != false {
		t.Error("expected failure for unknown tool")
	}
}
