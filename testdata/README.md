# logsee regression and performance checks

Run the correctness gates with `make verify`. The app tests include a conditional
PTY helper which is skipped during ordinary `go test` runs. To exercise the real
Bubble Tea event loop in a controlling terminal:

```sh
go test -c -o /tmp/logsee-app.test ./internal/adapter/app
python3 testdata/pty_smoke.py /tmp/logsee-app.test
```

The PTY test uses a temporary home directory and an injected clipboard writer. It
checks file and live pipe input, UTF-8, copy outside the viewport, search editing,
help, resize, quit, raw stdio preservation and terminal restoration. It answers
terminal color/cursor queries so initialization does not depend on a terminal
emulator. It does not write the desktop clipboard or the user's input history.

Run `make benchmark` for 10MiB, 100MiB and 1GiB generated files, filtered and
unfiltered, with follow enabled and disabled. Fixtures are temporary and the OS
page cache is warm after fixture creation. Each fixture can exceed its named size
by one write chunk (approximately 1MiB). The benchmark measures first frame
construction, complete initial indexing/filtering, 30 real cursor moves through
rendering, and bytes read for one append. Up disables follow; the benchmark
restores follow before measuring the following append. The first frame may show
`lines:0/?` while indexing continues. The generated line is 132 bytes long and
matches the test filter. Long-line assembly and large copy selections have
separate, input-dependent memory costs.

## Validation on 2026-11-20

Environment: Linux amd64, AMD Ryzen 9 9950X, Go 1.22.2, warm filesystem cache.
Results from one benchmark iteration per combination (milliseconds):

| Size | Full count, unfiltered | Full count + filter | Key-to-frame p95, worst combination |
| --- | ---: | ---: | ---: |
| 10MiB | 7.1–8.3 | 44.3–44.8 | 4.89 |
| 100MiB | 74.6–77.5 | 388.7–395.6 | 4.92 |
| 1GiB | 656.1–676.1 | 3710.0–3881.0 | 4.90 |

First frame construction: 0.105–0.175ms, excluding terminal initialization.
Append reads: 528–197,400 bytes across the scenarios; no whole-file rescan.
Retained Go heap after GC: approximately 0.6–6.0MiB across scenarios. The raw cache
holds at most three 64KiB pages; heap also includes output rows, metadata and Go
runtime state. Peak process RSS across the benchmark sweep was 22.3MiB
(`/usr/bin/time -v`, 22,824KiB). These are controlled benchmark observations, not latency guarantees
for all disks or inputs.

PTY live stdio storage/display: approximately 39ms in the final measured run. Both PTY cases restored
canonical/echo mode and the alternate screen. Clipboard correctness is covered by
the injected writer and adapter unit tests; Linux desktop xclip and macOS pbcopy
were not exercised against a live desktop clipboard.

## Review regression coverage

| Review ID | Correction | Main regression evidence |
| --- | --- | --- |
| B01 | Idle stdio batches flush after 50ms | `TestStdioIdleFlush`, PTY stream case |
| B02 | Wait for complete auto-type samples or EOF | `TestLiveTypeWaitsForCompleteSample`, delayed-start ADB test |
| B03 | Periodic file refresh, identity/truncation handling | `TestFileFollowRefreshesWithoutKey` |
| B04 | Incremental filter counts, sparse seeks, background jobs | `TestAppendFilterScansOnlyDelta`, `TestBackwardFilteredNavigationUsesMetadata`, benchmark |
| B05 | No Scanner token limit; drain data on read errors | `TestStdioLongLine`, `TestStdioReadErrorFlushesAcceptedData` |
| B06 | Copy selected source lines outside the viewport | `TestReviewCopyOutsideViewport`, PTY copy case |
| B07 | Cell widths and grapheme-safe clipping/wrapping | `TestDisplayCellWidth`, `TestWrappedSingleLineCanReachEnd` |
| B08 | Reassemble partial tail without creating a new raw line | `TestReviewPartialLineAppend`, `TestAppendRetainsRawCoordinates` |
| B09 | Reload expanded viewport on resize | `TestReviewResizeLoadsExpandedViewport`, PTY resize |
| B10 | Shift+Home/End and selection mode behavior | `TestReviewShiftHomeEndSelect`, selection/copy count test |
| B11 | First-token priority, disjoint highlight intervals | `TestReviewOverlappingColorsPreserveText` |
| B12 | Standards-compliant TOML decoder | `TestReviewReadmeConfig`, `TestTOMLRegexStringEscapes` |
| B13 | Extract KV values using original byte positions | `TestIgnoreCaseUnicodePrefix` |
| B14 | Runtime raw-page source with sparse metadata | `TestPageWindowBound`, `TestSourceSnapshotBounds` |
| B15 | Sync before publication; join before closing the sink | `TestStdioSyncFailureNotPublished`, `TestStdioPublishesOnlyAfterSync`, `TestStdioQuitDrainsAcceptedData` |

The legacy scripted renderer remains synchronous for deterministic compatibility
tests; the shipped CLI always uses the asynchronous Bubble Tea renderer. Blocking
custom readers without cancellation or Close support cannot be forcibly stopped;
the CLI's file descriptors use a cancellable reader. A long line and a requested
clipboard selection can require memory proportional to their text size.

## Independent review follow-up

The independent review identified five additional races/cancellation defects.
They were reproduced as failing tests and fixed in the same change:

- Reject obsolete selection/search intent after `/` or Esc.
- Preserve the original Shift anchor and the order of queued selection operations.
- Keep file-generation replacement pending until the UI acknowledges it.
- Check cancellation between metadata pages and in the real filesystem adapter.
- Retry interrupted append indexing from the successfully scanned frontier.

`final_review_test.go` in app/usecase contains the reproductions, including rapid
Shift contraction, ordinary movement after Shift and Space toggles. No review
findings were deferred. Uneven input-read sizes also have a regression ensuring
raw write batches never exceed 64KiB.

The implementation was reviewed as a working-tree diff on `fix/logsee-review`
in the requested checkout. The pre-existing deleted `main` binary was preserved. The execution
ledger is manual because task headings differ from the installed skill's parser
and the review preceded the commit request. Real timer tests use bounded deadlines instead of
an additional fake-clock dependency; unusually severe scheduler stalls can fail
those timing assertions. Integration/merge/publication remains a separate step.
