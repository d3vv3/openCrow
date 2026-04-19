package api

import "testing"

func TestNormalizeCreateMessageInput_AllowsAttachmentsOnly(t *testing.T) {
	role, content, attachments, err := normalizeCreateMessageInput(CreateMessageRequest{
		Role:    "user",
		Content: "   ",
		Attachments: []CreateMessageAttachmentRequest{{
			FileName: "notes.txt",
			MimeType: "text/plain",
			DataURL:  "data:text/plain;base64,SGVsbG8=",
		}},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if role != "user" {
		t.Fatalf("role = %q, want user", role)
	}
	if content != "" {
		t.Fatalf("content = %q, want empty", content)
	}
	if len(attachments) != 1 {
		t.Fatalf("attachments = %d, want 1", len(attachments))
	}
}

func TestNormalizeCreateMessageInput_RejectsInvalidAttachmentDataURL(t *testing.T) {
	_, _, _, err := normalizeCreateMessageInput(CreateMessageRequest{
		Role:    "user",
		Content: "hello",
		Attachments: []CreateMessageAttachmentRequest{{
			FileName: "notes.txt",
			MimeType: "text/plain",
			DataURL:  "https://example.com/notes.txt",
		}},
	})
	if err == nil || err.Error() != "attachment dataUrl must be a data URL" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNormalizeCreateMessageInput_RejectsAssistantRole(t *testing.T) {
	_, _, _, err := normalizeCreateMessageInput(CreateMessageRequest{Role: "assistant", Content: "hello"})
	if err == nil || err.Error() != "role required and content or attachments required" {
		t.Fatalf("unexpected error: %v", err)
	}
}
