package rules

import (
	"errors"
	"testing"
)

func TestLoadBadJSON(t *testing.T) {
	if _, err := load([]byte("{")); err == nil {
		t.Fatal("want error")
	}
}

func TestDataIntegrity(t *testing.T) {
	rs, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 425 {
		t.Errorf("retext-equality 6.6.0 has 425 patterns, got %d", len(rs))
	}
	ids, phrases := map[string]bool{}, map[string]string{}
	for _, r := range rs {
		if ids[r.ID] {
			t.Errorf("duplicate id %s", r.ID)
		}
		ids[r.ID] = true
		if r.Type != "basic" && r.Type != "or" {
			t.Errorf("%s: bad type %q", r.ID, r.Type)
		}
		for _, p := range r.Inconsiderate {
			if prev, ok := phrases[p[0]]; ok {
				t.Errorf("phrase %q in both %s and %s", p[0], prev, r.ID)
			}
			phrases[p[0]] = r.ID
		}
	}
}

func check(t *testing.T, text string, o Options) []Finding {
	t.Helper()
	rs, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	got, err := Check(rs, text, o)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestAllowAndDeny(t *testing.T) {
	text := "The whitelist and the blacklist"
	if n := len(check(t, text, Options{})); n != 2 {
		t.Fatalf("want 2 findings, got %d", n)
	}
	got := check(t, text, Options{Allow: []string{"whitelist", "blacklist"}})
	if len(got) != 0 {
		t.Errorf("allowed rule still reported: %+v", got)
	}
	got = check(t, text, Options{Deny: []string{"whitelist", "blacklist"}})
	if len(got) != 2 {
		t.Errorf("denied rule must still report: %+v", got)
	}
	got = check(t, text, Options{Deny: []string{"nothing"}})
	if len(got) != 0 {
		t.Errorf("deny reports only listed rules: %+v", got)
	}
}

func TestAllowAndDenyTogether(t *testing.T) {
	rs, _ := Load()
	_, err := Check(rs, "x", Options{Allow: []string{"a"}, Deny: []string{"b"}})
	if !errors.Is(err, ErrAllowDeny) {
		t.Fatalf("want ErrAllowDeny, got %v", err)
	}
}

func TestFindingFields(t *testing.T) {
	got := check(t, "ok\nTheir child has a birth defect.", Options{})
	if len(got) != 1 {
		t.Fatalf("got %+v", got)
	}
	f := got[0]
	if f.Line != 2 || f.Column != 19 || f.EndLine != 2 || f.EndColumn != 24 || f.RuleID != "birth-defect" ||
		f.Actual != "birth defect" || len(f.Expected) != 3 || f.Note == "" {
		t.Errorf("bad finding: %+v", f)
	}
}

func TestNoConsiderate(t *testing.T) {
	// Nine upstream patterns carry no suggestion.
	for _, r := range mustLoad(t) {
		if len(r.Considerate) == 0 {
			got := check(t, r.Inconsiderate[0][0], Options{})
			if len(got) == 0 || got[0].Expected != nil {
				t.Fatalf("%s: %+v", r.ID, got)
			}
			return
		}
	}
	t.Fatal("no pattern without suggestions")
}

func mustLoad(t *testing.T) []Rule {
	t.Helper()
	rs, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	return rs
}

func TestColumnsCountUTF16(t *testing.T) {
	got := check(t, "🙂 whitelist", Options{})
	if len(got) != 1 || got[0].Column != 4 {
		t.Fatalf("an astral character takes two UTF-16 units: %+v", got)
	}
}
