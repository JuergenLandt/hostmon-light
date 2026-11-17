package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
)

type config struct {
	listen, host, token, level, format string
	interval, age, header, write, idle time.Duration
	paths                              map[string]string
}

func env(k, d string) string {
	if v, ok := os.LookupEnv(k); ok {
		return v
	}
	return d
}
func duration(k, d string) (time.Duration, error) {
	v, e := time.ParseDuration(env(k, d))
	if e != nil || v <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", k)
	}
	return v, nil
}
func load() (c config, err error) {
	allowed := map[string]bool{}
	for _, k := range strings.Fields("LISTEN HOSTNAME TOKEN_FILE SAMPLE_INTERVAL MAX_SAMPLE_AGE HTTP_READ_HEADER_TIMEOUT HTTP_WRITE_TIMEOUT HTTP_IDLE_TIMEOUT FILESYSTEMS LOG_LEVEL LOG_FORMAT") {
		allowed["MONITOR_"+k] = true
	}
	for _, v := range os.Environ() {
		k := strings.SplitN(v, "=", 2)[0]
		if strings.HasPrefix(k, "MONITOR_") && !allowed[k] {
			return c, fmt.Errorf("unknown variable %s", k)
		}
	}
	c.listen = env("MONITOR_LISTEN", ":8080")
	c.host = env("MONITOR_HOSTNAME", "")
	if c.host == "" {
		return c, errors.New("MONITOR_HOSTNAME required")
	}
	p := env("MONITOR_TOKEN_FILE", "/run/secrets/api_token")
	b, e := os.ReadFile(p)
	if e != nil {
		return c, errors.New("cannot read token file")
	}
	c.token = strings.TrimSpace(string(b))
	if len(c.token) < 32 {
		return c, errors.New("token must contain at least 32 bytes")
	}
	for _, x := range []struct {
		k, d string
		p    *time.Duration
	}{{"MONITOR_SAMPLE_INTERVAL", "10s", &c.interval}, {"MONITOR_MAX_SAMPLE_AGE", "30s", &c.age}, {"MONITOR_HTTP_READ_HEADER_TIMEOUT", "5s", &c.header}, {"MONITOR_HTTP_WRITE_TIMEOUT", "5s", &c.write}, {"MONITOR_HTTP_IDLE_TIMEOUT", "30s", &c.idle}} {
		*x.p, e = duration(x.k, x.d)
		if e != nil {
			return c, e
		}
	}
	if c.interval < time.Second || c.age < 2*c.interval {
		return c, errors.New("interval must be >=1s and max age >= twice interval")
	}
	c.level = env("MONITOR_LOG_LEVEL", "info")
	c.format = env("MONITOR_LOG_FORMAT", "json")
	if c.level != "debug" && c.level != "info" && c.level != "warn" && c.level != "error" {
		return c, errors.New("invalid log level")
	}
	if c.format != "json" && c.format != "text" {
		return c, errors.New("invalid log format")
	}
	c.paths = map[string]string{}
	spec := env("MONITOR_FILESYSTEMS", "")
	if spec != "" {
		for _, s := range strings.Split(spec, ",") {
			kv := strings.SplitN(s, "=", 2)
			if len(kv) != 2 || kv[0] == "" || !strings.HasPrefix(kv[1], "/") {
				return c, errors.New("invalid filesystem specification")
			}
			if _, ok := c.paths[kv[0]]; ok {
				return c, errors.New("duplicate filesystem name")
			}
			c.paths[kv[0]] = kv[1]
		}
	}
	return c, nil
}

type snapshot struct {
	Schema      int                           `json:"schema_version"`
	Host        string                        `json:"host"`
	At          time.Time                     `json:"sampled_at"`
	Ready       bool                          `json:"ready"`
	CPU         *float64                      `json:"cpu_usage_percent"`
	CPUSeconds  float64                       `json:"cpu_sample_seconds"`
	Memory      *memoryMetrics                `json:"memory"`
	Swap        *swapMetrics                  `json:"swap"`
	Filesystems map[string]*filesystemMetrics `json:"filesystems"`
	Errors      map[string]string             `json:"errors"`
}
type store struct {
	sync.RWMutex
	s snapshot
}

func cpuDelta(a, b cpu.TimesStat) (float64, bool) {
	// Guest time is already included in user/nice on Linux.
	total := func(t cpu.TimesStat) float64 {
		return t.User + t.Nice + t.System + t.Idle + t.Iowait + t.Irq + t.Softirq + t.Steal
	}
	d := total(b) - total(a)
	idle := (b.Idle + b.Iowait) - (a.Idle + a.Iowait)
	if d <= 0 || idle < 0 {
		return 0, false
	}
	v := 100 * (d - idle) / d
	if v < 0 {
		v = 0
	}
	if v > 100 {
		v = 100
	}
	return v, true
}
func collect(ctx context.Context, c config, st *store) {
	var prev *cpu.TimesStat
	var prevAt time.Time
	lastState := ""
	run := func() {
		s := snapshot{Schema: 1, Host: c.host, At: time.Now().UTC(), Filesystems: map[string]*filesystemMetrics{}, Errors: map[string]string{}}
		t, e := cpu.TimesWithContext(ctx, false)
		now := time.Now()
		if e != nil || len(t) != 1 {
			s.Errors["cpu"] = "CPU counters unavailable"
			prev = nil
		} else {
			if prev != nil {
				v, ok := cpuDelta(*prev, t[0])
				if ok {
					s.CPU = &v
					s.CPUSeconds = now.Sub(prevAt).Seconds()
				} else {
					s.Errors["cpu"] = "CPU counters reset"
				}
			} else {
				s.Errors["cpu"] = "warming up"
			}
			v := t[0]
			prev = &v
			prevAt = now
		}
		mv, me := mem.VirtualMemoryWithContext(ctx)
		if me != nil || mv == nil || mv.Total == 0 {
			s.Errors["memory"] = "memory unavailable"
		} else {
			s.Memory = memoryValue(mv)
		}
		sv, se := mem.SwapMemoryWithContext(ctx)
		if se != nil || sv == nil {
			s.Errors["swap"] = "swap unavailable"
		} else {
			s.Swap = swapValue(sv)
		}
		for name, path := range c.paths {
			v, e := disk.UsageWithContext(ctx, path)
			if e != nil {
				s.Filesystems[name] = nil
				s.Errors["filesystem:"+name] = "filesystem unavailable"
			} else {
				s.Filesystems[name] = filesystemValue(v)
			}
		}
		s.Ready = len(s.Errors) == 0
		b, _ := json.Marshal(s.Errors)
		state := string(b)
		if state != lastState {
			if s.Ready {
				slog.Info("collection ready")
			} else {
				slog.Warn("collection degraded", "errors", s.Errors)
			}
			lastState = state
		}
		st.Lock()
		st.s = s
		st.Unlock()
	}
	run()
	tick := time.NewTicker(c.interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			run()
		}
	}
}
func handler(c config, st *store) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			w.WriteHeader(405)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		st.RLock()
		at := st.s.At
		st.RUnlock()
		if at.IsZero() || time.Since(at) > c.age {
			http.Error(w, "stale", 503)
			return
		}
		w.Write([]byte("ok\n"))
	})
	want := sha256.Sum256([]byte("Bearer " + c.token))
	mux.HandleFunc("/api/v1/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != "GET" {
			w.WriteHeader(405)
			return
		}
		got := sha256.Sum256([]byte(r.Header.Get("Authorization")))
		if subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", 401)
			return
		}
		st.RLock()
		s := st.s
		st.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		if s.At.IsZero() || time.Since(s.At) > c.age || !s.Ready {
			w.WriteHeader(503)
		}
		json.NewEncoder(w).Encode(s)
	})
	return mux
}
func main() {
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		u := "http://" + env("MONITOR_LISTEN", "127.0.0.1:8080") + "/healthz"
		cl := http.Client{Timeout: 2 * time.Second}
		r, e := cl.Get(u)
		if e != nil {
			os.Exit(1)
		}
		r.Body.Close()
		if r.StatusCode != 200 {
			os.Exit(1)
		}
		return
	}
	if os.Geteuid() == 0 {
		fmt.Fprintln(os.Stderr, "refusing to run as root")
		os.Exit(1)
	}
	c, e := load()
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	level := slog.LevelInfo
	level.UnmarshalText([]byte(c.level))
	opt := &slog.HandlerOptions{Level: level}
	var h slog.Handler = slog.NewJSONHandler(os.Stdout, opt)
	if c.format == "text" {
		h = slog.NewTextHandler(os.Stdout, opt)
	}
	slog.SetDefault(slog.New(h))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	st := &store{}
	go collect(ctx, c, st)
	srv := &http.Server{Addr: c.listen, Handler: handler(c, st), ReadHeaderTimeout: c.header, ReadTimeout: 10 * time.Second, WriteTimeout: c.write, IdleTimeout: c.idle, MaxHeaderBytes: 8192}
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(c)
	}()
	slog.Info("starting", "host", c.host, "listen", c.listen, "uid", strconv.Itoa(os.Geteuid()))
	if e = srv.ListenAndServe(); e != nil && !errors.Is(e, http.ErrServerClosed) {
		slog.Error("server stopped")
		os.Exit(1)
	}
}

type memoryMetrics struct {
	Total       uint64  `json:"total_bytes"`
	Available   uint64  `json:"available_bytes"`
	UsedPercent float64 `json:"used_percent"`
}
type swapMetrics struct {
	Total       uint64  `json:"total_bytes"`
	Used        uint64  `json:"used_bytes"`
	UsedPercent float64 `json:"used_percent"`
}
type filesystemMetrics struct {
	Path              string   `json:"path"`
	Total             uint64   `json:"total_bytes"`
	Available         uint64   `json:"available_bytes"`
	Used              uint64   `json:"used_bytes"`
	UsedPercent       float64  `json:"used_percent"`
	InodesTotal       uint64   `json:"inodes_total"`
	InodesAvailable   uint64   `json:"inodes_available"`
	InodesUsedPercent *float64 `json:"inodes_used_percent"`
}

func percent(n, d uint64) float64 {
	if d == 0 {
		return 0
	}
	p := 100 * float64(n) / float64(d)
	if p > 100 {
		return 100
	}
	return p
}
func memoryValue(v *mem.VirtualMemoryStat) *memoryMetrics {
	a := v.Available
	if a > v.Total {
		a = v.Total
	}
	return &memoryMetrics{v.Total, a, percent(v.Total-a, v.Total)}
}
func swapValue(v *mem.SwapMemoryStat) *swapMetrics {
	u := v.Used
	if u > v.Total {
		u = v.Total
	}
	return &swapMetrics{v.Total, u, percent(u, v.Total)}
}
func filesystemValue(v *disk.UsageStat) *filesystemMetrics {
	var ip *float64
	if v.InodesTotal > 0 {
		p := percent(v.InodesUsed, v.InodesTotal)
		ip = &p
	}
	den := float64(v.Used) + float64(v.Free)
	p := 0.0
	if den > 0 {
		p = 100 * float64(v.Used) / den
		if p > 100 {
			p = 100
		}
	}
	return &filesystemMetrics{v.Path, v.Total, v.Free, v.Used, p, v.InodesTotal, v.InodesFree, ip}
}
