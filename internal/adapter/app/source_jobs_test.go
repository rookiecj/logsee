package app

import (
	"context"
	"logsee/internal/usecase"
	"os"
	"testing"
	"time"
)

func TestFilterReplacementDropsStaleResult(t *testing.T) {
	s := reviewState(t)
	s.filterRevision = 2
	m := teaLoopModel{ctx: context.Background(), state: s, jobs: &sourceJobs{}, requestID: 2}
	next, _ := m.Update(sourceResult{ID: 1, FilterRevision: 1, Records: []usecase.OutputLogRecord{{RawLineNumber: 9, Text: "stale"}}})
	if next.(teaLoopModel).state.records[0].Text == "stale" {
		t.Fatal("obsolete filter result applied")
	}
}

type blockingSource struct{ started chan struct{} }

func (s *blockingSource) Path() string                        { return "blocked" }
func (s *blockingSource) Size(context.Context) (int64, error) { return 4, nil }
func (s *blockingSource) ReadAt(ctx context.Context, _ int64, _ int) ([]byte, error) {
	select {
	case <-s.started:
	default:
		close(s.started)
	}
	<-ctx.Done()
	return nil, ctx.Err()
}
func TestQuitCancelsSlowScan(t *testing.T) {
	path := writeLoopLog(t, "one\n")
	raw := &blockingSource{started: make(chan struct{})}
	jobs := newSourceJobsWithSource(context.Background(), path, raw)
	jobs.Submit(sourceRequest{ID: 1, LogType: usecase.LogTypePlain, Selection: usecase.NewSelectionState()})
	<-raw.started
	done := make(chan struct{})
	go func() { jobs.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("active source scan did not stop")
	}
}
func TestFileFollowRefreshesWithoutKey(t *testing.T) {
	path := writeLoopLog(t, "one\n")
	jobs := newSourceJobs(context.Background(), path)
	defer jobs.Close()
	req := sourceRequest{ID: 1, LogType: usecase.LogTypePlain, Navigation: usecase.NavigationOptions{ViewportHeight: 2, Follow: true}, Selection: usecase.NewSelectionState()}
	jobs.Submit(req)
	first := <-jobs.results
	if first.Err != nil {
		t.Fatal(first.Err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	file.WriteString("two\nthree\n")
	file.Close()
	req.ID++
	jobs.Submit(req)
	next := <-jobs.results
	if next.Err != nil || next.TotalRaw != 3 || next.Navigation.CursorOutputIndex() != 2 || next.Records[len(next.Records)-1].Text != "three" {
		t.Fatalf("append result=%+v", next)
	}
	// Replacement at the same byte length must invalidate source coordinates.
	replacement := path + ".new"
	os.WriteFile(replacement, []byte("new\nnew\nnew\n"), 0600)
	os.Rename(replacement, path)
	req.ID++
	jobs.Submit(req)
	next = <-jobs.results
	if next.Err != nil || !next.Replaced || next.Records[0].Text != "new" {
		t.Fatalf("replacement result=%+v", next)
	}
}
