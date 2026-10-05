package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestDryRunsNeedNoCredentials(t *testing.T) {
	id := "00000000-0000-0000-0000-000000000003"
	cases := [][]string{{"instance", "list", "--dry-run"}, {"instance", "get", id, "--dry-run"}, {"instance", "metrics", id, "--dry-run"}, {"instance", "start", id}, {"instance", "stop", id}, {"instance", "start", id, "--execute", "--dry-run"}, {"image", "list", "--private", "--dry-run"}, {"dataset", "list", "--dry-run"}, {"storage", "list", "--idc", "test-idc", "--dry-run"}}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			c := New("test")
			var b bytes.Buffer
			c.SetOut(&b)
			c.SetArgs(args)
			if err := c.Execute(); err != nil {
				t.Fatal(err)
			}
			var got map[string]any
			if json.Unmarshal(b.Bytes(), &got) != nil || got["dry_run"] != true {
				t.Fatalf("invalid dry run %s", b.String())
			}
			if strings.Contains(b.String(), "Authorization") {
				t.Fatal("dry run includes auth")
			}
		})
	}
}

func TestUsageAndHelpOffline(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"instance", "--help"}, {"completion", "powershell"}} {
		c := New("test")
		c.SetOut(&bytes.Buffer{})
		c.SetArgs(args)
		if err := c.Execute(); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"instance", "stop", "bad"}, {"instance", "list", "--page", "0", "--dry-run"}, {"dataset", "list", "--page-size", "1000", "--dry-run"}, {"storage", "list"}, {"instance", "start", "00000000-0000-0000-0000-000000000003", "--mode", "BAD"}, {"instance", "list", "--timeout", "0s", "--dry-run"}} {
		c := New("test")
		c.SetOut(&bytes.Buffer{})
		c.SetArgs(args)
		if err := c.Execute(); err == nil {
			t.Fatalf("invalid args accepted: %v", args)
		}
	}
}

func TestTableControlCharacters(t *testing.T) {
	if got := cell(map[string]any{"name": "unsafe\x1b[31m\t\nname"}, "name"); strings.ContainsAny(got, "\x1b\t\n") {
		t.Fatal("terminal control injection")
	}
}
