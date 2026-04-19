package classify

import (
	"strconv"

	"git.inpt.fr/42dottools/log/internal/analysis"
	"git.inpt.fr/42dottools/log/internal/domain"
)

// Classifier applies a list of Rules to each Record and emits a Finding
// for the first matching rule. Stateless; safe to share across goroutines
// (the pipeline still routes one writer per analyzer).
type Classifier struct {
	rules []Rule
}

// New returns a Classifier backed by the built-in rule set.
func New() *Classifier { return &Classifier{rules: Rules()} }

// NewWithRules returns a Classifier over a caller-supplied rule table —
// use when loading rules from disk or overriding for tests.
func NewWithRules(rules []Rule) *Classifier {
	cp := make([]Rule, len(rules))
	copy(cp, rules)
	return &Classifier{rules: cp}
}

func (c *Classifier) Name() string { return "classify" }

// OnRecord evaluates rules in order and returns the first match as a
// Finding. The finding carries Seq, Kind, Severity, and a small Fields
// map (tag, pid) so downstream consumers have context without re-joining
// against the Records index.
func (c *Classifier) OnRecord(r domain.Record) analysis.Output {
	for _, rl := range c.rules {
		if !rl.Match(r) {
			continue
		}
		return analysis.Output{Findings: []domain.Finding{buildFinding(rl, r)}}
	}
	return analysis.Output{}
}

// Flush has no deferred state in the stateless classifier.
func (c *Classifier) Flush() analysis.Output { return analysis.Output{} }

func buildFinding(rl Rule, r domain.Record) domain.Finding {
	f := domain.Finding{
		Kind:       rl.Kind,
		Seq:        r.Seq,
		Severity:   rl.Severity,
		Confidence: 1.0,
		SchemaVer:  domain.SchemaVersion,
	}
	fields := make(map[string]string, 2)
	if r.Tag != "" {
		fields["tag"] = r.Tag
	}
	if r.PID != 0 {
		fields["pid"] = strconv.FormatInt(int64(r.PID), 10)
	}
	if len(fields) > 0 {
		f.Fields = fields
	}
	return f
}
