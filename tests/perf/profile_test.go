//go:build perf

// Package perf_test contains opt-in profiling tests for performance analysis.
//
// These tests are intentionally excluded from the normal test suite. They seed
// large datasets, run sustained workloads, and write pprof snapshots to disk
// for later analysis with go tool pprof. Run them explicitly:
//
//	task test-perf
//	  or
//	go test -tags perf -v -run TestPerf ./tests/perf/
//
// Profile files are written to tests/perf/profiles/ and are gitignored.
// Analyse them with:
//
//	go tool pprof tests/perf/profiles/heap-<timestamp>.pprof
//	go tool pprof tests/perf/profiles/profile-<timestamp>.pprof
package perf_test

import (
	"net/http"
	"testing"
	"time"
)

// seedCount is the number of objects seeded into the test bucket.
// 10 000 objects gives a realistic large-bucket workload. Lower it
// if you want a quicker smoke run: PERF_SEED_COUNT=1000 has no effect
// yet — change the constant if you need a smaller run.
const seedCount = 10_000

// TestPerfMetadataCaching profiles the metadata access pattern during
// ListObjectsV2. With the current implementation every listed object
// triggers an individual GetObjectMetadata disk read. The CPU profile
// captured here shows that dominating the call stack, which motivates
// the in-memory caching work.
//
// What to look at in the profile:
//
//	go tool pprof -http=: tests/perf/profiles/profile-*.pprof
//	# focus on storage.(*Storage).listInternal and metadata.(*Manager).GetObjectMetadata
func TestPerfMetadataCaching(t *testing.T) {
	ps := newPerfServer(t)

	t.Log("seeding bucket ...")
	seedBucket(t, ps, "bench-meta", seedCount)
	t.Log("seeding complete")

	// Baseline heap — before any listing work.
	captureProfile(t, ps, "heap", 0)

	// Warm up the OS page cache and Go runtime to get stable CPU readings.
	for i := 0; i < 3; i++ {
		ps.listObjectsV2(t, "bench-meta", "", 1000)
	}

	// Drive sustained listing while a CPU profile is collected.
	// The goroutine runs in the background; captureProfile blocks for
	// `seconds` seconds collecting the sample.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			ps.listObjectsV2(t, "bench-meta", "", 1000)
		}
	}()

	cpu := captureProfile(t, ps, "profile", 10)
	<-done

	// Post-workload heap — compare allocations to the baseline.
	heap := captureProfile(t, ps, "heap", 0)

	t.Logf("\nAnalyse metadata caching profiles:\n"+
		"  go tool pprof -http=: %s\n"+
		"  go tool pprof -http=: %s", cpu, heap)
}

// TestPerfListObjectsLargeBucket measures ListObjectsV2 latency at different
// page sizes and prefix filters against a 10 000-object bucket.
//
// What to look at:
//
//	The logged per-call timings show where latency grows non-linearly.
//	The CPU profile captures the walk + sort + filter hot path.
func TestPerfListObjectsLargeBucket(t *testing.T) {
	ps := newPerfServer(t)

	t.Log("seeding bucket ...")
	seedBucket(t, ps, "bench-list", seedCount)
	t.Log("seeding complete")

	cases := []struct {
		name    string
		prefix  string
		maxKeys int
	}{
		{"full-scan-1000", "", 1000},
		{"full-scan-100", "", 100},
		{"prefix-flat-1000", "flat/", 1000},
		{"prefix-a-1000", "prefix-a/", 1000},
		{"prefix-deep-1000", "deep/", 1000},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			const iterations = 20

			start := time.Now()
			for i := 0; i < iterations; i++ {
				status := ps.listObjectsV2(t, "bench-list", tc.prefix, tc.maxKeys)
				if status != http.StatusOK {
					t.Errorf("unexpected status %d on iteration %d", status, i)
				}
			}
			elapsed := time.Since(start)
			avg := elapsed / iterations

			t.Logf("prefix=%q max-keys=%d  avg=%s  total=%s",
				tc.prefix, tc.maxKeys, avg, elapsed)
		})
	}

	// CPU profile across a mixed workload.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 50; i++ {
			ps.listObjectsV2(t, "bench-list", "", 1000)
			ps.listObjectsV2(t, "bench-list", "flat/", 1000)
		}
	}()
	cpu := captureProfile(t, ps, "profile", 10)
	<-done

	t.Logf("\nAnalyse list-objects profile:\n  go tool pprof -http=: %s", cpu)
}

// TestPerfMemory looks for memory growth across repeated ListObjectsV2 calls.
// It takes a heap snapshot before the workload and after, then logs both paths
// so you can diff them in pprof.
//
// What to look at:
//
//	go tool pprof -http=: tests/perf/profiles/heap-<before>.pprof
//	go tool pprof -http=: tests/perf/profiles/heap-<after>.pprof
//
//	Or use the pprof base flag to diff:
//	go tool pprof -http=: -base tests/perf/profiles/heap-<before>.pprof \
//	    tests/perf/profiles/heap-<after>.pprof
func TestPerfMemory(t *testing.T) {
	ps := newPerfServer(t)

	t.Log("seeding bucket ...")
	seedBucket(t, ps, "bench-mem", seedCount)
	t.Log("seeding complete")

	before := captureProfile(t, ps, "heap", 0)
	goroutinesBefore := captureProfile(t, ps, "goroutine", 0)

	// Run a sustained mixed workload: full scans + prefix filters.
	const rounds = 200
	for i := 0; i < rounds; i++ {
		ps.listObjectsV2(t, "bench-mem", "", 1000)
		if i%4 == 0 {
			ps.listObjectsV2(t, "bench-mem", "flat/", 1000)
		}
		if i%8 == 0 {
			ps.listObjectsV2(t, "bench-mem", "deep/", 1000)
		}
	}

	after := captureProfile(t, ps, "heap", 0)
	goroutinesAfter := captureProfile(t, ps, "goroutine", 0)
	allocs := captureProfile(t, ps, "allocs", 0)

	t.Logf("\nAnalyse memory profiles:\n"+
		"  # Diff heap before/after:\n"+
		"  go tool pprof -http=: -base %s %s\n\n"+
		"  # Allocations during workload:\n"+
		"  go tool pprof -http=: %s\n\n"+
		"  # Goroutine counts (check for leaks):\n"+
		"  go tool pprof -http=: -base %s %s",
		before, after,
		allocs,
		goroutinesBefore, goroutinesAfter,
	)
}
