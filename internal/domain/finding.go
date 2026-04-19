package domain

// FindingKind is the identifier space for detected anomalies. Values
// encode the rule or detector that produced the Finding; analyzers pick
// from this enum rather than inventing their own strings so cross-tool
// aggregation stays consistent.
type FindingKind uint16

const (
	FindingUnknown FindingKind = iota

	// Tier A — deterministic signatures (rule-based, high precision).
	// Android adb log origin.
	FindingFatalJava
	FindingANR
	FindingNativeCrashHeader
	FindingWatchdog
	FindingLMKKill
	FindingBinderFail
	FindingSELinuxDenied
	FindingWTF
	FindingOOM

	// Systemd/Linux kernel journal origin.
	FindingSystemdUnitFailed
	FindingSystemdCoredump
	FindingKernelPanic
	FindingKernelBUG
	FindingSegfaultLinux
	FindingOOMKilledLinux
	FindingAppArmorDenied
	FindingSSHAuthFailure

	// Tier B — stateful behavioural anomalies.
	FindingGCStorm
	FindingWakelockLeak
	FindingBootLoop

	// Tier C — statistical anomalies.
	FindingRareTemplate
	FindingBurst
	FindingBaselineDeviation
)

var findingKindNames = [...]string{
	FindingUnknown:           "unknown",
	FindingFatalJava:         "fatal_java",
	FindingANR:               "anr",
	FindingNativeCrashHeader: "native_crash_header",
	FindingWatchdog:          "watchdog",
	FindingLMKKill:           "lmk_kill",
	FindingBinderFail:        "binder_fail",
	FindingSELinuxDenied:     "selinux_denied",
	FindingWTF:               "wtf",
	FindingOOM:               "oom",
	FindingSystemdUnitFailed: "systemd_unit_failed",
	FindingSystemdCoredump:   "systemd_coredump",
	FindingKernelPanic:       "kernel_panic",
	FindingKernelBUG:         "kernel_bug",
	FindingSegfaultLinux:     "segfault",
	FindingOOMKilledLinux:    "oom_killed",
	FindingAppArmorDenied:    "apparmor_denied",
	FindingSSHAuthFailure:    "ssh_auth_failure",
	FindingGCStorm:           "gc_storm",
	FindingWakelockLeak:      "wakelock_leak",
	FindingBootLoop:          "boot_loop",
	FindingRareTemplate:      "rare_template",
	FindingBurst:             "burst",
	FindingBaselineDeviation: "baseline_deviation",
}

func (k FindingKind) String() string {
	if int(k) >= len(findingKindNames) {
		return "unknown"
	}
	return findingKindNames[k]
}

func (k FindingKind) MarshalText() ([]byte, error) { return []byte(k.String()), nil }

func (k *FindingKind) UnmarshalText(b []byte) error {
	for i, name := range findingKindNames {
		if name == string(b) {
			*k = FindingKind(i)
			return nil
		}
	}
	*k = FindingUnknown
	return nil
}

// Finding is one anomaly detected by an analyzer. SpanID is 0 for single
// line findings; when set, the finding is the authoritative marker for
// its span and analyzers should not emit both a standalone finding and
// the span-anchored one for the same event.
type Finding struct {
	Kind       FindingKind       `json:"kind"`
	Seq        Seq               `json:"seq"`
	SpanID     int64             `json:"span_id,omitempty"`
	Severity   Level             `json:"severity,omitempty"`
	Confidence float32           `json:"confidence,omitempty"`
	Fields     map[string]string `json:"fields,omitempty"`
	SchemaVer  uint16            `json:"schema_version,omitempty"`
}
