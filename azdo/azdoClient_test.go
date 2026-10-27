package azdo

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMakeHTTPRequestRejectsErrorStatus(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError} {
		server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			response.WriteHeader(status)
			_, _ = response.Write([]byte(`{"message":"request failed"}`))
		}))
		client := AzDoClient{Client: server.Client()}
		request, err := http.NewRequest(http.MethodGet, server.URL, nil)
		if err != nil {
			server.Close()
			t.Fatal(err)
		}
		_, err = client.makeHTTPRequest(request)
		server.Close()
		if err == nil {
			t.Errorf("HTTP %d was treated as successful data", status)
		}
	}
}
