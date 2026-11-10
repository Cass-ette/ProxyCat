package controller

import "testing"

func TestSocketOwnersUseExactProtocolAndAddress(t *testing.T) {
	owners := parseSocketOwners(`p123
cBrowser
PTCP
n127.0.0.1:51234->127.0.0.1:7890
PUDP
n127.0.0.1:51234->1.1.1.1:53
PTCP
n[::1]:51235->[::1]:7890
n*:7890
p456
cOther
PTCP
n192.0.2.1:51234->203.0.113.1:443
`)
	for _, key := range []string{socketKey("tcp", "127.0.0.1", "51234"), socketKey("udp", "127.0.0.1", "51234"), socketKey("tcp", "::1", "51235")} {
		if owners[key].pid != "123" {
			t.Errorf("lost socket owner for %s", key)
		}
	}
	if owners[socketKey("tcp", "192.0.2.1", "51234")].pid != "456" {
		t.Fatal("confused same port on different addresses")
	}
	if len(owners) != 4 {
		t.Fatal("listener treated as an outgoing socket")
	}
	if socketKey("tcp", "*", "7890") != "" || socketKey("tcp", "127.0.0.1", "invalid") != "" {
		t.Fatal("accepted incomplete endpoint")
	}
}

func TestSocketOwnersRejectAmbiguousApplications(t *testing.T) {
	owners := parseSocketOwners("p123\ncBrowser\nPTCP\nn127.0.0.1:51234->127.0.0.1:7890\np456\ncOther\nPTCP\nn127.0.0.1:51234->203.0.113.1:443\np123\ncBrowser\nPTCP\nn127.0.0.1:51234->127.0.0.1:7890\n")
	if owners[socketKey("tcp", "127.0.0.1", "51234")].pid != "" {
		t.Fatal("guessed an owner for a reused port")
	}
}

func TestProcessPathsPreserveBundleNamesWithSpaces(t *testing.T) {
	paths := parseProcessPaths("  123 /Applications/Example Browser.app/Contents/Frameworks/Browser Helper.app/Contents/MacOS/Helper\n456 /usr/bin/curl\nnot-a-pid ignored\n")
	if connectionApp(paths["123"], "Helper") != "Example Browser" || connectionApp(paths["456"], "curl") != "curl" || len(paths) != 2 {
		t.Fatal("lost application path or accepted non-PID output")
	}
}
