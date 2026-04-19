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
	{
		Kind: domain.FindingFatalJava, Severity: domain.LevelFatal,
		TagEq:       "AndroidRuntime",
		MsgPrefix:   "*** FATAL EXCEPTION IN SYSTEM PROCESS",
		Description: "system_server Java fatal",
	},
	{
		Kind: domain.FindingFatalJava, Severity: domain.LevelFatal,
		TagEq:       "AndroidRuntime",
		MsgPrefix:   "FATAL EXCEPTION",
		Description: "Java uncaught exception",
	},
	{
		Kind: domain.FindingANR, Severity: domain.LevelError,
		TagEq:       "ActivityManager",
		MsgPrefix:   "ANR in",
		Description: "Application not responding",
	},
	{
		Kind: domain.FindingNativeCrashHeader, Severity: domain.LevelFatal,
		TagEq:       "DEBUG",
		MsgRegex:    regexp.MustCompile(`^\*\*\*\s+\*\*\*\s+\*\*\*`),
		Description: "Native tombstone header",
	},
	{
		Kind: domain.FindingWatchdog, Severity: domain.LevelFatal,
		TagEq:       "Watchdog",
		MsgContains: []string{"WATCHDOG KILLING", "Blocked in"},
		Description: "Watchdog killed or blocked the system",
	},
	{
		Kind: domain.FindingLMKKill, Severity: domain.LevelWarn,
		TagEq:       "lowmemorykiller",
		MsgContains: []string{"Killing"},
		Description: "Low memory killer reaping a process",
	},
	{
		Kind: domain.FindingBinderFail, Severity: domain.LevelError,
		MsgContains: []string{"FAILED BINDER TRANSACTION", "TransactionTooLargeException"},
		Description: "Binder IPC failure",
	},
	{
		Kind: domain.FindingSELinuxDenied, Severity: domain.LevelWarn,
		MsgContains: []string{"avc: denied"},
		Description: "SELinux access denied",
	},
	{
		Kind: domain.FindingWTF, Severity: domain.LevelError,
		MsgContains: []string{"Log.wtf", "wtf_"},
		Description: "WTF log emission",
	},
	{
		Kind: domain.FindingOOM, Severity: domain.LevelFatal,
		MsgContains: []string{"OutOfMemoryError"},
		Description: "Java out-of-memory",
	},
}
