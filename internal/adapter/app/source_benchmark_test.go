package app

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"logsee/internal/adapter/filesystem"
	"logsee/internal/usecase"
)

type countedFile struct {
	filesystem.FileSource
	bytes int64
}

func (f *countedFile) ReadAt(ctx context.Context, offset int64, size int) ([]byte, error) {
	data, err := f.FileSource.ReadAt(ctx, offset, size)
	f.bytes += int64(len(data))
	return data, err
}

// Run explicitly with -run '^$' -bench BenchmarkSourcePipeline -benchtime=1x.
// Fixture writes warm the OS page cache; this is not a cold-cache benchmark.
func BenchmarkSourcePipeline(b *testing.B) {
	for _, mb := range []int{10, 100, 1024} {
		b.Run(fmt.Sprintf("%dMiB", mb), func(b *testing.B) {
			path := b.TempDir() + "/large.log"
			file, err := os.Create(path)
			if err != nil {
				b.Fatal(err)
			}
			line := "11-20 12:34:56.789  123  456 E Tag: service=prod error " + strings.Repeat("x", 76) + "\n"
			chunk := []byte(strings.Repeat(line, 8192))
			size := int64(0)
			for size < int64(mb)*1024*1024 {
				n, e := file.Write(chunk)
				if e != nil {
					b.Fatal(e)
				}
				size += int64(n)
			}
			file.Close()
			for _, filtered := range []bool{false, true} {
				for _, follow := range []bool{false, true} {
					b.Run(fmt.Sprintf("filter=%v/follow=%v", filtered, follow), func(b *testing.B) {
						ctx := context.Background()
						raw := &countedFile{FileSource: filesystem.NewFileSource(path)}
						jobs := newSourceJobsWithSource(ctx, path, raw)
						defer jobs.Close()
						began := time.Now()
						s, e := newLoopState(ctx, usecase.InputSession{Mode: usecase.InputModeFile, SOTPath: path}, path, usecase.LogTypeADB, 100, 24, asyncRecordLimit, "")
						if e != nil {
							b.Fatal(e)
						}
						if filtered {
							s.filterEditingText = "service:prod"
							s.applyFilterEditingText()
						}
						if follow {
							s.navigation.Move(usecase.NavigationMoveLastAndFollow)
						}
						m := teaLoopModel{ctx: ctx, state: s, jobs: jobs}
						m.submitSource()
						_ = m.View()
						firstFrame := time.Since(began)
						result := <-jobs.results
						if result.Err != nil {
							b.Fatal(result.Err)
						}
						next, _ := m.Update(result)
						m = next.(teaLoopModel)
						indexed := time.Since(began)
						// Place the non-follow cursor in the middle so every Up is a real move.
						if !follow {
							s.navigation, _ = usecase.NewNavigationState(usecase.NavigationOptions{OutputCount: s.outputCount(), ViewportHeight: s.listHeight, CursorOutputIndex: s.outputCount() / 2, ScrollOffset: s.outputCount() / 2})
							m.submitSource()
							result = <-jobs.results
							if result.Err != nil {
								b.Fatal(result.Err)
							}
							next, _ = m.Update(result)
							m = next.(teaLoopModel)
						}
						latencies := make([]time.Duration, 0, 30)
						for i := 0; i < 30; i++ {
							began := time.Now()
							next, _ = m.Update(tea.KeyMsg(tea.Key{Type: tea.KeyUp}))
							m = next.(teaLoopModel)
							if m.busy {
								result = <-jobs.results
								if result.Err != nil {
									b.Fatal(result.Err)
								}
								next, _ = m.Update(result)
								m = next.(teaLoopModel)
							}
							_ = m.View()
							latencies = append(latencies, time.Since(began))
						}
						sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
						// Up stops follow by design; restore it before measuring a following append.
						if follow {
							s.navigation.Move(usecase.NavigationMoveLastAndFollow)
							m.submitSource()
							result = <-jobs.results
							next, _ = m.Update(result)
							m = next.(teaLoopModel)
						}
						before := raw.bytes
						f, e := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
						if e != nil {
							b.Fatal(e)
						}
						f.WriteString(line)
						f.Sync()
						f.Close()
						m.submitSource()
						result = <-jobs.results
						if result.Err != nil {
							b.Fatal(result.Err)
						}
						appendRead := raw.bytes - before
						runtime.GC()
						var mem runtime.MemStats
						runtime.ReadMemStats(&mem)
						b.ReportMetric(float64(firstFrame.Microseconds())/1000, "first-frame-ms")
						b.ReportMetric(float64(indexed.Microseconds())/1000, "indexed-ms")
						b.ReportMetric(float64(latencies[28].Microseconds())/1000, "key-p95-ms")
						b.ReportMetric(float64(appendRead), "append-read-B")
						b.ReportMetric(float64(mem.HeapAlloc)/1024/1024, "heap-MiB")
					})
				}
			}
		})
	}
}
