// Package classify applies deterministic rules (Tier A signatures) to each
// incoming Record and emits one Finding per matching rule. The rule table
// lives in code for now; a TOML loader can swap it out in a later phase
// without touching the Classifier.
package classify

import (
	"regexp"
	"strings"

	"git.inpt.fr/42dottools/log/internal/domain"
)

// Rule is one detector entry. Predicates AND together: an empty predicate
// passes. Keep each check cheap (string ops) because the Classifier runs
// every rule against every record.
type Rule struct {
	Kind     domain.FindingKind
	Severity domain.Level

	// Formats limits which LineFormats this rule applies to. Empty means
	// any format; typically Android-origin rules list Android and journal
	// rules list journal so they never fire on each other's lines.
	Formats []domain.LineFormat

	// TagEq requires Record.Tag to equal this exactly. Empty = any tag.
	TagEq string

	// MsgPrefix requires the (space-trimmed) message to start with this.
	// Empty = no prefix check.
	MsgPrefix string

	// MsgContains requires at least one of these substrings to appear in
	// the raw message. Empty = no substring check.
	MsgContains []string

	// MsgRegex matches the raw message. Nil = no regex check.
	MsgRegex *regexp.Regexp

	// Description is for docs and debug logging; never written to the wire.
	Description string
}

// Match reports whether r satisfies every configured predicate.
func (rl Rule) Match(r domain.Record) bool {
	if len(rl.Formats) > 0 {
		ok := false
		for _, f := range rl.Formats {
			if f == r.Format {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if rl.TagEq != "" && r.Tag != rl.TagEq {
		return false
	}
	if rl.MsgPrefix != "" && !strings.HasPrefix(strings.TrimSpace(r.Message), rl.MsgPrefix) {
		return false
	}
	if len(rl.MsgContains) > 0 {
		hit := false
		for _, s := range rl.MsgContains {
			if strings.Contains(r.Message, s) {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	if rl.MsgRegex != nil && !rl.MsgRegex.MatchString(r.Message) {
		return false
	}
	return true
}

// Rules returns a fresh copy of the built-in Tier A rule table. Order is
// evaluation preference: when multiple rules would match the same record,
// the classifier stops at the first match to avoid double-counting.
func Rules() []Rule {
	return append([]Rule(nil), builtinRules...)
}

var builtinRules = []Rule{
	// --- Android adb ---
	{
		Kind: domain.FindingFatalJava, Severity: domain.LevelFatal,
		Formats:     []domain.LineFormat{domain.LineFormatAndroid},
		TagEq:       "AndroidRuntime",
		MsgPrefix:   "*** FATAL EXCEPTION IN SYSTEM PROCESS",
		Description: "system_server Java fatal",
	},
	{
		Kind: domain.FindingFatalJava, Severity: domain.LevelFatal,
		Formats:     []domain.LineFormat{domain.LineFormatAndroid},
		TagEq:       "AndroidRuntime",
		MsgPrefix:   "FATAL EXCEPTION",
		Description: "Java uncaught exception",
	},
	{
		Kind: domain.FindingANR, Severity: domain.LevelError,
		Formats:     []domain.LineFormat{domain.LineFormatAndroid},
		TagEq:       "ActivityManager",
		MsgPrefix:   "ANR in",
		Description: "Application not responding",
	},
	{
		Kind: domain.FindingNativeCrashHeader, Severity: domain.LevelFatal,
		Formats:     []domain.LineFormat{domain.LineFormatAndroid},
		TagEq:       "DEBUG",
		MsgRegex:    regexp.MustCompile(`^\*\*\*\s+\*\*\*\s+\*\*\*`),
		Description: "Native tombstone header",
	},
	{
		Kind: domain.FindingWatchdog, Severity: domain.LevelFatal,
		Formats:     []domain.LineFormat{domain.LineFormatAndroid},
		TagEq:       "Watchdog",
		MsgContains: []string{"WATCHDOG KILLING", "Blocked in"},
		Description: "Watchdog killed or blocked the system",
	},
	{
		Kind: domain.FindingLMKKill, Severity: domain.LevelWarn,
		Formats:     []domain.LineFormat{domain.LineFormatAndroid},
		TagEq:       "lowmemorykiller",
		MsgContains: []string{"Killing"},
		Description: "Low memory killer reaping a process",
	},
	{
		Kind: domain.FindingBinderFail, Severity: domain.LevelError,
		Formats:     []domain.LineFormat{domain.LineFormatAndroid},
		MsgContains: []string{"FAILED BINDER TRANSACTION", "TransactionTooLargeException"},
		Description: "Binder IPC failure",
	},
	{
		// SELinux avc: denied pattern is shared — same message in both
		// Android adb and Linux journal. Leave Formats empty to apply to
		// both.
		Kind: domain.FindingSELinuxDenied, Severity: domain.LevelWarn,
		MsgContains: []string{"avc: denied"},
		Description: "SELinux access denied",
	},
	{
		Kind: domain.FindingWTF, Severity: domain.LevelError,
		Formats:     []domain.LineFormat{domain.LineFormatAndroid},
		MsgContains: []string{"Log.wtf", "wtf_"},
		Description: "WTF log emission",
	},
	{
		Kind: domain.FindingOOM, Severity: domain.LevelFatal,
		Formats:     []domain.LineFormat{domain.LineFormatAndroid},
		MsgContains: []string{"OutOfMemoryError"},
		Description: "Java out-of-memory",
	},

	// --- systemd / Linux kernel (journalctl short-iso) ---
	{
		Kind: domain.FindingSystemdUnitFailed, Severity: domain.LevelError,
		Formats:     []domain.LineFormat{domain.LineFormatJournal},
		TagEq:       "systemd",
		MsgContains: []string{"Failed with result", "Main process exited, code=dumped", "Start request repeated too quickly"},
		Description: "systemd unit entered failed state",
	},
	{
		Kind: domain.FindingSystemdCoredump, Severity: domain.LevelError,
		Formats:     []domain.LineFormat{domain.LineFormatJournal},
		TagEq:       "systemd-coredump",
		MsgContains: []string{"dumped core", "Coredump diverted"},
		Description: "systemd-coredump captured a crash",
	},
	{
		Kind: domain.FindingKernelPanic, Severity: domain.LevelFatal,
		Formats:     []domain.LineFormat{domain.LineFormatJournal},
		TagEq:       "kernel",
		MsgContains: []string{"Kernel panic"},
		Description: "Linux kernel panic",
	},
	{
		Kind: domain.FindingKernelBUG, Severity: domain.LevelFatal,
		Formats:     []domain.LineFormat{domain.LineFormatJournal},
		TagEq:       "kernel",
		MsgRegex:    regexp.MustCompile(`^(BUG:|Oops:)`),
		Description: "Linux kernel BUG/Oops",
	},
	{
		Kind: domain.FindingSegfaultLinux, Severity: domain.LevelError,
		Formats:     []domain.LineFormat{domain.LineFormatJournal},
		TagEq:       "kernel",
		MsgContains: []string{"segfault at"},
		Description: "userspace process segfaulted (reported by kernel)",
	},
	{
		Kind: domain.FindingOOMKilledLinux, Severity: domain.LevelError,
		Formats:     []domain.LineFormat{domain.LineFormatJournal},
		TagEq:       "kernel",
		MsgContains: []string{"Out of memory: Killed process", "invoked oom-killer"},
		Description: "Linux OOM killer reaped a process",
	},
	{
		Kind: domain.FindingAppArmorDenied, Severity: domain.LevelWarn,
		Formats:     []domain.LineFormat{domain.LineFormatJournal},
		MsgContains: []string{`apparmor="DENIED"`},
		Description: "AppArmor denied an operation",
	},
	{
		Kind: domain.FindingSSHAuthFailure, Severity: domain.LevelWarn,
		Formats:     []domain.LineFormat{domain.LineFormatJournal},
		TagEq:       "sshd",
		MsgContains: []string{"Failed password for"},
		Description: "sshd authentication failure",
	},
}
