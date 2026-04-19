package classify

import (
	"bufio"
	"os"
	"path/filepath"
	"testing"

	"git.inpt.fr/42dottools/log/internal/domain"
	"git.inpt.fr/42dottools/log/internal/pipeline"
)

// Each journal sample must produce at least one finding of the expected
// kind. Keeps the rule table honest against realistic noise.
func TestClassifier_DetectsJournalKinds(t *testing.T) {
	cases := []struct {
		sample string
		want   domain.FindingKind
	}{
		{"systemd_unit_failed.log", domain.FindingSystemdUnitFailed},
		{"kernel_panic.log", domain.FindingKernelPanic},
		{"kernel_panic.log", domain.FindingKernelBUG},
		{"oom_killer.log", domain.FindingOOMKilledLinux},
		{"coredump.log", domain.FindingSystemdCoredump},
		{"coredump.log", domain.FindingSegfaultLinux},
		{"auth_failures.log", domain.FindingSSHAuthFailure},
		{"auth_failures.log", domain.FindingAppArmorDenied},
		{"auth_failures.log", domain.FindingSELinuxDenied},
	}
	for _, tc := range cases {
		t.Run(tc.sample+"/"+tc.want.String(), func(t *testing.T) {
			findings := classifyJournalSample(t, tc.sample)
			for _, f := range findings {
				if f.Kind == tc.want {
					return
				}
			}
			kinds := map[domain.FindingKind]int{}
			for _, f := range findings {
				kinds[f.Kind]++
			}
			t.Errorf("sample %s: no %v finding; histogram=%v", tc.sample, tc.want, kinds)
		})
	}
}

// Cross-format guard: journal samples must not trigger Android Tier A rules
// (and the inverse is covered by the existing Android tests).
func TestClassifier_JournalDoesNotTriggerAndroidRules(t *testing.T) {
	androidOnly := map[domain.FindingKind]bool{
		domain.FindingFatalJava:         true,
		domain.FindingANR:               true,
		domain.FindingNativeCrashHeader: true,
		domain.FindingWatchdog:          true,
		domain.FindingLMKKill:           true,
		domain.FindingBinderFail:        true,
		domain.FindingWTF:               true,
		domain.FindingOOM:               true,
	}
	samples := []string{"systemd_unit_failed.log", "kernel_panic.log", "oom_killer.log", "coredump.log", "auth_failures.log"}
	for _, s := range samples {
		findings := classifyJournalSample(t, s)
		for _, f := range findings {
			if androidOnly[f.Kind] {
				t.Errorf("sample %s leaked Android-only finding kind %v at seq %d", s, f.Kind, f.Seq)
			}
		}
	}
}

func classifyJournalSample(t *testing.T, name string) []domain.Finding {
	t.Helper()
	path := filepath.Join("..", "..", "..", "testdata", "journalctl", name)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()

	c := New()
	var findings []domain.Finding
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var seq int64
	for scanner.Scan() {
		seq++
		line := domain.Line{Seq: seq, Text: scanner.Text()}
		r := pipeline.BuildRecord(line, domain.LineFormatJournal)
		findings = append(findings, c.OnRecord(r).Findings...)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan %s: %v", path, err)
	}
	findings = append(findings, c.Flush().Findings...)
	return findings
}
