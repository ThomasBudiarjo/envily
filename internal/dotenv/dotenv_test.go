package dotenv

import (
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	src := "# comment\r\n" +
		"export A=1\n" +
		"B = plain value # trailing comment\n" +
		"C='lit $HOME \\n #x'\n" +
		"D=\"line1\\nline2 \\\"q\\\"\"\n" +
		"E=\"multi\n" +
		"line\"\n" +
		"F=\n" +
		"G=a#b\n"
	got, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	want := []Entry{
		{"A", "1"},
		{"B", "plain value"},
		{"C", `lit $HOME \n #x`},
		{"D", "line1\nline2 \"q\""},
		{"E", "multi\nline"},
		{"F", ""},
		{"G", "a#b"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v\nwant %#v", got, want)
	}
}

func TestParseErrors(t *testing.T) {
	for _, src := range []string{"NOEQUALS", "1BAD=x", "A=\"open", "A='open"} {
		if _, err := Parse(src); err == nil {
			t.Errorf("Parse(%q): expected error", src)
		}
	}
}

func TestFormatRoundTrip(t *testing.T) {
	in := []Entry{
		{"PLAIN", "abc"},
		{"EMPTY", ""},
		{"SPACES", " a b "},
		{"DOLLAR", "p@$$w0rd"},
		{"SINGLE", "it's"},
		{"MULTI", "-----BEGIN KEY-----\nabc\\n\"x\"\n-----END KEY-----"},
		{"HASH", "a #b"},
	}
	out, err := Parse(Format(in))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(out, in) {
		t.Fatalf("round trip mismatch:\n%s\ngot %#v", Format(in), out)
	}
}
