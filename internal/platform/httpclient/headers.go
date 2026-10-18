package httpclient

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// ParseHeaders reads static headers in the OTEL_EXPORTER_OTLP_HEADERS format: "name1=value1,name2=value2".
// Values are percent-decoded, so a value can hold "," or "=" as %2C / %3D. "+" stays "+" (base64 tokens).
// An empty string gives an empty header.
func ParseHeaders(raw string) (http.Header, error) {
	header := http.Header{}
	if strings.TrimSpace(raw) == "" {
		return header, nil
	}
	for entry := range strings.SplitSeq(raw, ",") {
		name, value, found := strings.Cut(entry, "=")
		name = strings.TrimSpace(name)
		if !found || name == "" {
			return nil, fmt.Errorf("header entry '%s' must have the form 'name=value'", entry)
		}
		decoded, err := url.PathUnescape(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("header '%s' has a malformed percent-encoded value: %w", name, err)
		}
		if decoded == "" {
			return nil, fmt.Errorf("header '%s' has an empty value", name)
		}
		header.Add(name, decoded)
	}
	return header, nil
}

// headerTransport adds static headers to each request, e.g. auth at a proxy.
// It must be the innermost transport, so that the dump and otel never see the header values.
type headerTransport struct {
	next   http.RoundTripper
	header http.Header
}

func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// a RoundTripper must not change the request of the caller
	req = req.Clone(req.Context())
	for name, values := range t.header {
		for _, v := range values {
			req.Header.Add(name, v)
		}
	}
	return t.next.RoundTrip(req)
}

// withHeaders puts a headerTransport in front of next; no header gives next unchanged.
func withHeaders(next http.RoundTripper, header http.Header) http.RoundTripper {
	if len(header) == 0 {
		return next
	}
	return &headerTransport{next: next, header: header}
}
