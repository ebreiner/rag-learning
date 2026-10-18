package httpclient

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func TestParseHeaders(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    http.Header
		wantErr bool
	}{
		{"empty", "", http.Header{}, false},
		{"blank", "  ", http.Header{}, false},
		{"one", "X-Api-Key=secret", http.Header{"X-Api-Key": {"secret"}}, false},
		{"two with spaces", " x-api-key = a , Foo=b ", http.Header{"X-Api-Key": {"a"}, "Foo": {"b"}}, false},
		{"percent-encoded", "Authorization=Bearer%20abc", http.Header{"Authorization": {"Bearer abc"}}, false},
		{"base64 keeps plus and padding", "X-Api-Key=ab+c/d==", http.Header{"X-Api-Key": {"ab+c/d=="}}, false},
		{"encoded comma", "Foo=a%2Cb", http.Header{"Foo": {"a,b"}}, false},
		{"repeated name", "Foo=a,Foo=b", http.Header{"Foo": {"a", "b"}}, false},
		{"no equals", "X-Api-Key", nil, true},
		{"empty name", "=secret", nil, true},
		{"empty value", "X-Api-Key=", nil, true},
		{"trailing comma", "Foo=a,", nil, true},
		{"bad percent", "Foo=%zz", nil, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseHeaders(tc.raw)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if !tc.wantErr && !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNewSendsHeadersWithoutChangingTheRequest(t *testing.T) {
	var got http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
	}))
	defer server.Close()

	client := New(time.Minute, http.Header{"X-Api-Key": {"secret"}})
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "multipart/form-data")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if v := got.Get("X-Api-Key"); v != "secret" {
		t.Errorf("server got X-Api-Key %q, want %q", v, "secret")
	}
	if v := got.Get("Content-Type"); v != "multipart/form-data" {
		t.Errorf("server got Content-Type %q, want the header of the caller", v)
	}
	if v := req.Header.Get("X-Api-Key"); v != "" {
		t.Errorf("request of the caller changed: X-Api-Key %q", v)
	}
}

func TestNewWithoutHeaders(t *testing.T) {
	client := New(time.Minute, nil)
	if _, ok := client.Transport.(*headerTransport); ok {
		t.Fatal("want no headerTransport for a nil header")
	}
	var got http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
	}))
	defer server.Close()

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if v := got.Get("X-Api-Key"); v != "" {
		t.Errorf("server got X-Api-Key %q, want empty", v)
	}
}
