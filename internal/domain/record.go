package domain

import "time"

// Record is the parsed L1 view of a Line: one log entry with the fields
// the analysis layer expects. It references the source line by Seq only
// — analyzers that need the raw text look up the Line via Seq.
//
// Zero values are valid: a line whose format cannot be parsed yields a
// Record with LevelUnknown and empty Tag/Component, letting downstream
// analyzers decide whether to skip it or apply fallback rules.
type Record struct {
	Seq       Seq        `json:"seq"`
	Time      time.Time  `json:"time,omitempty"`
	Level     Level      `json:"level"`
	PID       int32      `json:"pid,omitempty"`
	TID       int32      `json:"tid,omitempty"`
	Tag       string     `json:"tag,omitempty"`
	Component string     `json:"component,omitempty"`
	Message   string     `json:"msg,omitempty"`
	Format    LineFormat `json:"fmt,omitempty"`
	SchemaVer uint16     `json:"schema_version,omitempty"`
}
