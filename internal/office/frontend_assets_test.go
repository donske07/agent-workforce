package office

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFrontendDeclaresInlineFavicon(t *testing.T) {
	// Given
	server := httptest.NewServer(NewHandler())
	defer server.Close()

	// When
	resp, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	// Then
	if !strings.Contains(string(data), `rel="icon"`) {
		t.Fatal("frontend must declare a favicon")
	}
}
