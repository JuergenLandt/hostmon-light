package main

import (
	"github.com/shirou/gopsutil/v4/cpu"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCPUDelta(t *testing.T) {
	v, ok := cpuDelta(cpu.TimesStat{User: 10, Idle: 10}, cpu.TimesStat{User: 20, Idle: 40})
	if !ok || v != 25 {
		t.Fatalf("got %v %v", v, ok)
	}
}
func TestAPIAuth(t *testing.T) {
	c := config{token: "test", age: time.Minute}
	st := &store{s: snapshot{At: time.Now(), Ready: true}}
	h := handler(c, st)
	for _, x := range []struct {
		token string
		code  int
	}{{"", 401}, {"Bearer wrong", 401}, {"Bearer test", 200}} {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil)
		r.Header.Set("Authorization", x.token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != x.code {
			t.Fatalf("got %d want %d", w.Code, x.code)
		}
	}
}
func TestStale(t *testing.T) {
	h := handler(config{age: time.Second}, &store{})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/healthz", nil))
	if w.Code != 503 {
		t.Fatal(w.Code)
	}
}
