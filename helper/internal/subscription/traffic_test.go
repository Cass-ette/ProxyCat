package subscription

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTrafficHeaderRequiresCompleteNonNegativeCounters(t *testing.T) {
	for _, input := range []string{"", "total=100", "upload=0;download=-1;total=100", "upload=0;download=1;total=NaN", "upload=0;download=1;total=9223372036854775808", "upload=0;upload=1;download=2;total=100"} {
		if result := ParseTraffic(input); result != nil {
			t.Fatalf("unusable header became a known balance: %q %+v", input, result)
		}
	}
	result := ParseTraffic(" Upload = 10 ; download=20; total=100; expire=2000000000; future=ignored")
	if result == nil || result.Upload != 10 || result.Download != 20 || result.Total != 100 || result.Expire != 2000000000 {
		t.Fatalf("valid account counters lost: %+v", result)
	}
	if result := ParseTraffic("upload=0;download=0;total=0"); result == nil || result.Total != 0 {
		t.Fatalf("zero allowance must remain explicit, not invented as unlimited: %+v", result)
	}
}

func TestMetadataFetchDoesNotRequireConfigurationBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != ProbeUserAgents[0] {
			t.Error("missing compatible subscription user agent")
		}
		w.Header().Set("Subscription-Userinfo", "upload=10; download=20; total=100")
		w.Write([]byte("this is deliberately not a usable proxy config"))
	}))
	defer server.Close()
	result, err := FetchTraffic(server.Client(), server.URL)
	if err != nil || result == nil || result.Total != 100 {
		t.Fatalf("metadata-only fetch: %+v %v", result, err)
	}
}

func TestProbeKeepsTrafficFromTheSelectedSubscriptionResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == ProbeUserAgents[0] {
			w.Header().Set("Subscription-Userinfo", "upload=1; download=2; total=300")
			w.Write([]byte(validClashYAML("A", "B")))
		} else {
			w.Header().Set("Subscription-Userinfo", "upload=9; download=9; total=999")
			w.Write([]byte(validClashYAML("C")))
		}
	}))
	defer server.Close()
	result := Probe(server.Client(), server.URL)
	if result.Selected == nil || result.Traffic == nil || result.Traffic.Total != 300 {
		t.Fatalf("traffic did not follow the selected response: %+v", result.Traffic)
	}
}
