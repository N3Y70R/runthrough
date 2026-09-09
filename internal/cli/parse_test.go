// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"flag"
	"io"
	"reflect"
	"testing"
)

// A test session lost time to this: `up web-front --catalog X` silently
// dropped the flag and then reported "no catalog found", sending the reader
// to debug a catalog that was perfectly fine.
func TestParseAcceptsFlagsAfterPositionalArguments(t *testing.T) {
	fs, catalog, build := fixture()

	rest, err := parse(fs, []string{"web-front", "--build", "--catalog", "/x"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if *catalog != "/x" {
		t.Errorf("catalog: got %q, want /x", *catalog)
	}
	if !*build {
		t.Error("build: the boolean flag after a positional was dropped")
	}
	if want := []string{"web-front"}; !reflect.DeepEqual(rest, want) {
		t.Errorf("positionals: got %v, want %v", rest, want)
	}
}

func TestParseKeepsWorkingWithFlagsFirst(t *testing.T) {
	fs, catalog, build := fixture()

	rest, err := parse(fs, []string{"--catalog", "/x", "--build", "web-front", "gateway"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if *catalog != "/x" || !*build {
		t.Errorf("flags: catalog=%q build=%v", *catalog, *build)
	}
	if want := []string{"web-front", "gateway"}; !reflect.DeepEqual(rest, want) {
		t.Errorf("positionals: got %v, want %v", rest, want)
	}
}

func TestParseHandlesEqualsFormAndTerminator(t *testing.T) {
	fs, catalog, _ := fixture()

	rest, err := parse(fs, []string{"svc", "--catalog=/y", "--", "--not-a-flag"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if *catalog != "/y" {
		t.Errorf("catalog: got %q, want /y", *catalog)
	}
	if want := []string{"svc", "--not-a-flag"}; !reflect.DeepEqual(rest, want) {
		t.Errorf("positionals: got %v, want %v", rest, want)
	}
}

func fixture() (*flag.FlagSet, *string, *bool) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	catalog := fs.String("catalog", "", "")
	build := fs.Bool("build", false, "")
	return fs, catalog, build
}
