package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormalizeDAVError_HTMLLoginPage(t *testing.T) {
	err := normalizeDAVError(
		assertErr("HTTP multi-status request failed: 200 OK"),
		"https://mail.univention.de/ajax/share/abc",
		davHTTPObservation{StatusCode: http.StatusOK, ContentType: "text/html; charset=utf-8"},
	)
	if !strings.Contains(err, "web login or share page") {
		t.Fatalf("expected login/share hint, got %q", err)
	}
	if !strings.Contains(err, "/ajax/share/") {
		t.Fatalf("expected ajax/share hint, got %q", err)
	}
	if strings.Contains(err, "HTTP multi-status request failed: 200 OK") {
		t.Fatalf("expected normalized message, got %q", err)
	}
}

func TestNormalizeDAVError_NotFoundCalDAV(t *testing.T) {
	err := normalizeDAVError(
		assertErr("propfind failed: 404 Not Found"),
		"https://mail.univention.de/caldav/foo",
		davHTTPObservation{StatusCode: http.StatusNotFound},
	)
	if !strings.Contains(err, "HTTP 404 Not Found") {
		t.Fatalf("expected 404 explanation, got %q", err)
	}
	if !strings.Contains(err, "/caldav/") {
		t.Fatalf("expected caldav hint, got %q", err)
	}
}

func TestHandleTestDAVConnection_RequiresURL(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/v1/dav/test", strings.NewReader(`{"username":"alice"}`))
	w := httptest.NewRecorder()

	s.handleTestDAVConnection(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "url required") {
		t.Fatalf("body = %q", w.Body.String())
	}
}

func TestHandleTestDAVConnection_NormalizesEndpointErrors(t *testing.T) {
	htmlSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html><body>Login</body></html>"))
	}))
	defer htmlSrv.Close()

	s := &Server{}
	body := `{"url":"` + htmlSrv.URL + `/ajax/share/abc","username":"alice","password":"secret"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/dav/test", strings.NewReader(body))
	w := httptest.NewRecorder()

	s.handleTestDAVConnection(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var res DAVTestResult
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if res.OK {
		t.Fatalf("expected failed DAV test, got %+v", res)
	}
	if !strings.Contains(strings.ToLower(res.Error), "dav endpoint") {
		t.Fatalf("expected normalized DAV endpoint error, got %q", res.Error)
	}
	if !strings.Contains(res.Error, "/ajax/share/") {
		t.Fatalf("expected endpoint hint, got %q", res.Error)
	}
}

func assertErr(msg string) error {
	return &staticErr{msg: msg}
}

type staticErr struct{ msg string }

func (e *staticErr) Error() string { return e.msg }
