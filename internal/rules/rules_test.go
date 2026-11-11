package rules

import (
	"strings"
	"testing"
)

func TestCheck(t *testing.T) {
	rs, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	got := Check(rs, "ok\nadd it to the whitelist, guys", Options{Maybe: true})
	if len(got) != 2 {
		t.Fatalf("want 2 findings, got %d: %+v", len(got), got)
	}
	if got[0].Line != 2 || got[0].RuleID != "whitelist" || !got[0].Fatal {
		t.Errorf("bad first finding: %+v", got[0])
	}
	if got[1].Fatal {
		t.Errorf("maybe finding must not be fatal: %+v", got[1])
	}
}

func TestDataIntegrity(t *testing.T) {
	rs, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	terms := map[string]string{}
	for _, r := range rs {
		if r.ID == "" || len(r.Terms) == 0 || len(r.Considerate) == 0 {
			t.Errorf("incomplete rule: %+v", r)
		}
		if r.Severity < 1 || r.Severity > 2 {
			t.Errorf("%s: bad severity %d", r.ID, r.Severity)
		}
		if ids[r.ID] {
			t.Errorf("duplicate id %s", r.ID)
		}
		ids[r.ID] = true
		for _, term := range r.Terms {
			k := strings.ToLower(term)
			if prev, ok := terms[k]; ok {
				t.Errorf("term %q in both %s and %s", term, prev, r.ID)
			}
			terms[k] = r.ID
		}
	}
}

func TestLoadBadJSON(t *testing.T) {
	if _, err := load([]byte("{")); err == nil {
		t.Fatal("want error")
	}
}

func TestMessages(t *testing.T) {
	rs := []Rule{
		{ID: "none", Terms: []string{"foo"}, Severity: 2},
		{ID: "one", Terms: []string{"bar"}, Considerate: []string{"baz"}, Severity: 2},
	}
	rs = compile(rs)
	got := Check(rs, "foo bar\nbar", Options{})
	want := []string{
		"`foo` may be insensitive",
		"`bar` may be insensitive, use `baz` instead",
		"`bar` may be insensitive, use `baz` instead",
	}
	if len(got) != 3 {
		t.Fatalf("got %+v", got)
	}
	for i, w := range want {
		if got[i].Message != w {
			t.Errorf("%d: got %q want %q", i, got[i].Message, w)
		}
	}
	if got[0].Column != 1 || got[1].Column != 5 || got[2].Line != 2 {
		t.Errorf("bad order: %+v", got)
	}
}

func TestDeny(t *testing.T) {
	rs, _ := Load()
	got := Check(rs, "hi guys", Options{Maybe: true, Deny: []string{"guys"}})
	if len(got) != 1 || !got[0].Fatal {
		t.Fatalf("got %+v", got)
	}
}

func TestJoinOr(t *testing.T) {
	if got := joinOr([]string{"a", "b", "c"}); got != "`a`, `b` or `c`" {
		t.Errorf("got %q", got)
	}
	if got := joinOr([]string{"a"}); got != "`a`" {
		t.Errorf("got %q", got)
	}
}

func TestAllowAndMaybe(t *testing.T) {
	rs, _ := Load()
	if n := len(Check(rs, "guys", Options{})); n != 0 {
		t.Errorf("low-confidence hidden by default, got %d", n)
	}
	if n := len(Check(rs, "whitelist", Options{Allow: []string{"whitelist"}})); n != 0 {
		t.Errorf("allowed term still reported, got %d", n)
	}
}
