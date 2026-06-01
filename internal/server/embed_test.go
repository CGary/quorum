package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAssetHandler_ServesIndex(t *testing.T) {
	handler := AssetHandler()
	req, err := http.NewRequest(http.MethodGet, "/", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	if ct := rr.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("handler returned wrong content type: got %v want %v", ct, "text/html; charset=utf-8")
	}

	body := rr.Body.String()
	if len(body) == 0 {
		t.Errorf("handler returned empty body")
	}
}
