package thirdparty

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPusher_SendJSON_OK(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") == "key" && r.Header.Get("X-Signature") != "" {
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`{"ok":true}`))
			return
		}
		w.WriteHeader(401)
	}))
	defer ts.Close()

	p := NewPusher(nil, "key", "secret")
	code, body, err := p.SendJSON(context.Background(), ts.URL+"/hook", map[string]any{"x": 1})
	if err != nil || code != 200 {
		t.Fatalf("unexpected: code=%d err=%v", code, err)
	}
	if string(body) == "" {
		t.Fatalf("empty body")
	}
}
