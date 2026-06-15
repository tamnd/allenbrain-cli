package allenbrain

import (
	"testing"

	"github.com/tamnd/any-cli/kit"
)

// These tests are offline: they exercise the URI driver's pure string
// functions and the host wiring (mint, body, resolve), which need no network.
// The client's HTTP behaviour is covered in allenbrain_test.go.

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "allenbrain" {
		t.Errorf("Scheme = %q, want allenbrain", info.Scheme)
	}
	if len(info.Hosts) == 0 {
		t.Errorf("Hosts is empty")
	}
	found := false
	for _, h := range info.Hosts {
		if h == Host {
			found = true
		}
	}
	if !found {
		t.Errorf("Hosts does not contain %q: %v", Host, info.Hosts)
	}
	if info.Identity.Binary != "allenbrain" {
		t.Errorf("Identity.Binary = %q, want allenbrain", info.Identity.Binary)
	}
}

func TestClassifyGene(t *testing.T) {
	cases := []struct {
		in  string
		typ string
		id  string
		ok  bool
	}{
		{"42", "gene", "42", true},
		{"1234", "gene", "1234", true},
		{"allenbrain://gene/99", "gene", "99", true},
		{"allenbrain://atlas/3", "atlas", "3", true},
		{"allenbrain://dataset/77", "dataset", "77", true},
		{"not-a-number", "", "", false},
		{"", "", "", false},
	}
	for _, tc := range cases {
		typ, id, err := Domain{}.Classify(tc.in)
		if tc.ok {
			if err != nil {
				t.Errorf("Classify(%q) returned error: %v", tc.in, err)
				continue
			}
			if typ != tc.typ || id != tc.id {
				t.Errorf("Classify(%q) = (%q, %q), want (%q, %q)", tc.in, typ, id, tc.typ, tc.id)
			}
		} else {
			if err == nil {
				t.Errorf("Classify(%q) expected error, got (%q, %q, nil)", tc.in, typ, id)
			}
		}
	}
}

func TestLocate(t *testing.T) {
	cases := []struct {
		typ  string
		id   string
		want string
	}{
		{"gene", "1", BrainURL + "/gene/show/1"},
		{"atlas", "2", BrainURL + "/atlas/show/2"},
		{"dataset", "100", BrainURL + "/experiment/show/100"},
	}
	for _, tc := range cases {
		got, err := Domain{}.Locate(tc.typ, tc.id)
		if err != nil {
			t.Errorf("Locate(%q, %q) error: %v", tc.typ, tc.id, err)
			continue
		}
		if got != tc.want {
			t.Errorf("Locate(%q, %q) = %q, want %q", tc.typ, tc.id, got, tc.want)
		}
	}
}

func TestLocateUnknownType(t *testing.T) {
	_, err := Domain{}.Locate("bogus", "1")
	if err == nil {
		t.Error("expected error for unknown resource type, got nil")
	}
}

func TestHostWiring(t *testing.T) {
	h, err := kit.Open()
	if err != nil {
		t.Fatal(err)
	}

	g := &Gene{ID: 1, Acronym: "A1BG", Name: "alpha-1-B glycoprotein"}
	u, err := h.Mint(g)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	want := "allenbrain://gene/1"
	if u.String() != want {
		t.Errorf("Mint = %q, want %q", u.String(), want)
	}

	got, err := h.ResolveOn("allenbrain", "42")
	if err != nil {
		t.Fatalf("ResolveOn: %v", err)
	}
	if got.String() != "allenbrain://gene/42" {
		t.Errorf("ResolveOn = %q, want allenbrain://gene/42", got.String())
	}
}
