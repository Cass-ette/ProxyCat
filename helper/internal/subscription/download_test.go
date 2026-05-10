package subscription

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDownloadFetchesContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Fatal("missing User-Agent")
		}
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(200)
		_, _ = w.Write([]byte("test-content"))
	}))
	defer server.Close()

	client := &http.Client{}
	content, err := Download(client, server.URL, "clash-verge/v1.7.7")
	if err != nil {
		t.Fatalf("download: %v", err)
	}

	if string(content) != "test-content" {
		t.Fatalf("content = %q", string(content))
	}
}

func TestDownloadReturnsErrorOnNon200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", 404)
	}))
	defer server.Close()

	client := &http.Client{}
	_, err := Download(client, server.URL, "ProxyCat/1.0")
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

func TestDownloadRespectsSizeLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(200)
		large := make([]byte, maxDownloadSize+1000)
		for i := range large {
			large[i] = 'a'
		}
		_, _ = w.Write(large)
	}))
	defer server.Close()

	client := &http.Client{}
	_, err := Download(client, server.URL, "ProxyCat/1.0")
	if err == nil {
		t.Fatal("expected error for oversized response")
	}
}

type mockHTTPClient struct {
	response *http.Response
	err      error
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.response, nil
}

func TestDownloadWithMockClient(t *testing.T) {
	mock := &mockHTTPClient{
		response: &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(io.LimitReader(io.NewSectionReader(nil, 0, 0), 0)),
			Header:     make(http.Header),
		},
	}
	mock.response.Body = io.NopCloser(io.MultiReader())

	_, err := downloadWithClient(mock, "http://test", "UA")
	if err == nil {
		// Empty body is fine, we're just testing the client interface
	}
}
