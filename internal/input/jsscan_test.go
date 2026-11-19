package input

import "testing"

func TestJSOpen(t *testing.T) {
	for _, c := range []struct {
		code string
		open bool
	}{
		{"import a from 'a'", false},
		{"export const a = {\n  b: 1,", true},
		{"export const a = {\n  b: 1,\n}", false},
		{"export const f = (\n  1", true},
		{"export const a = `x", true},
		{"export const a = `x ${ {b: 1}.b } y`", false},
		{"export const a = 1 /* x", true},
		{"export const a = 1 /* x */", false},
		{"export const a = 1 // {", false},
		{"export const a = '{'", false},
		{"export const a = /[{]/", false},
		{"export const a = b / 2 // {", false},
		{"export const A = () => <div>", true},
		{"export const A = () => <div>{'}'}</div>", false},
		{"export const A = () => <><b a={1} /></>", false},
		{"export const A = () => <div a=\"{\">", true},
	} {
		if got := jsOpen(c.code); got != c.open {
			t.Errorf("jsOpen(%q) = %v, want %v", c.code, got, c.open)
		}
	}
}
