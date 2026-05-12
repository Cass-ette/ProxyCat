package selfupdate

import "testing"

func TestParseVersionAcceptsStrictMajorMinorPatch(t *testing.T) {
	v, err := parseVersion("0.12.3")
	if err != nil {
		t.Fatalf("parseVersion returned error: %v", err)
	}
	if v.major != 0 || v.minor != 12 || v.patch != 3 {
		t.Fatalf("version = %+v", v)
	}
}

func TestParseVersionRejectsNonStrictVersions(t *testing.T) {
	for _, input := range []string{"v0.1.0", "0.1", "0.1.0-beta.1", "latest", ""} {
		if _, err := parseVersion(input); err == nil {
			t.Fatalf("parseVersion(%q) returned nil error", input)
		}
	}
}

func TestVersionCompare(t *testing.T) {
	cases := []struct {
		current string
		latest  string
		want    int
	}{
		{"0.1.0", "0.1.1", -1},
		{"0.2.0", "0.1.99", 1},
		{"1.0.0", "1.0.0", 0},
	}
	for _, tc := range cases {
		current, _ := parseVersion(tc.current)
		latest, _ := parseVersion(tc.latest)
		if got := current.compare(latest); got != tc.want {
			t.Fatalf("%s compare %s = %d, want %d", tc.current, tc.latest, got, tc.want)
		}
	}
}
