package filter

import "testing"

func TestExtractRawLevel_plainNeverMatches(t *testing.T) {
	// Given: plain format and lines that would match adb/bracket patterns
	// When: ExtractRawLevel
	// Then: ok is false
	adb := `2022-12-29 04:00:18.823 30249-30321 Tag com.example D msg`
	bracket := `[2026-01-01T00:00:00Z] INFO: hello`
	for _, line := range []string{adb, bracket} {
		_, ok := ExtractRawLevel(line, FormatPlain)
		if ok {
			t.Fatalf("plain format should not extract level from %q", line)
		}
	}
}

func TestEffectiveFormatFromDetect_unknownAndBracketBecomePlain(t *testing.T) {
	if EffectiveFormatFromDetect(FormatUnknown) != FormatPlain {
		t.Fatal("unknown detection should map to plain")
	}
	if EffectiveFormatFromDetect(FormatBracket) != FormatPlain {
		t.Fatal("bracket should map to plain for user log type")
	}
	if EffectiveFormatFromDetect(FormatAndroid) != FormatAndroid {
		t.Fatal("android preserved")
	}
}

func TestMatch_levelPlainNoStructuredLevel(t *testing.T) {
	// Given: filter with +level:DEBUG and plain format
	prog, err := Parse(`+level:DEBUG`)
	if err != nil {
		t.Fatal(err)
	}
	line := `2022-12-29 04:00:18.823 1-2 Tag com.example D msg`
	// Then: no extracted level → positive include fails
	if Match(line, prog, false, FormatPlain) {
		t.Fatal("expected no match without structured level on plain")
	}
}
