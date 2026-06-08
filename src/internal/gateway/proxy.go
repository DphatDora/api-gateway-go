package gateway

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"

	"api-gateway/config"
	"api-gateway/package/logger"
)

const maxBodyLogBytes = 64 * 1024 // 64 KB cap per body read

// bodyPool reuses []byte buffers to reduce GC pressure when reading bodies.
var bodyPool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, 4096)
		return &b
	},
}

// readBody reads up to maxBodyLogBytes from r using a pooled buffer.
// Returns the bytes read and a restored ReadCloser to replace the original.
// The caller must set the body back on the request/response after calling this.
func readBody(rc io.ReadCloser) ([]byte, io.ReadCloser, error) {
	ptr := bodyPool.Get().(*[]byte)
	buf := (*ptr)[:0] // reuse capacity, reset length

	limited := io.LimitReader(rc, maxBodyLogBytes)
	var err error
	buf, err = io.ReadAll(limited) // ReadAll grows buf as needed
	_ = rc.Close()

	if err != nil {
		bodyPool.Put(ptr) //nolint:staticcheck // return even on error
		return nil, io.NopCloser(bytes.NewReader(nil)), err
	}

	// Copy bytes out of the pooled slice so the restored body is independent.
	out := make([]byte, len(buf))
	copy(out, buf)

	// Return buffer to pool
	*ptr = buf[:0]
	bodyPool.Put(ptr)

	return out, io.NopCloser(bytes.NewReader(out)), nil
}

// compactIfJSON returns a compacted JSON string if data is valid JSON,
// otherwise returns the original string unchanged.
func compactIfJSON(data []byte) string {
	var buf bytes.Buffer
	if json.Compact(&buf, data) == nil {
		return buf.String()
	}
	return string(data)
}

// isLoggableContentType reports whether the Content-Type is JSON or plain text.
func isLoggableContentType(ct string) bool {
	ct = strings.ToLower(ct)
	return strings.Contains(ct, "application/json") ||
		strings.Contains(ct, "text/plain") ||
		strings.Contains(ct, "text/html") ||
		strings.Contains(ct, "application/x-www-form-urlencoded")
}

// ServiceProxy holds a reverse proxy for a single downstream service.
type ServiceProxy struct {
	Name   string
	Prefix string
	Target *url.URL
	Proxy  *httputil.ReverseProxy
}

// NewServiceProxy creates a reverse proxy for the given service target.
func NewServiceProxy(svc config.ServiceTarget) (*ServiceProxy, error) {
	targetURL, err := url.Parse(svc.Target)
	if err != nil {
		return nil, err
	}

	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			// Strip the gateway prefix and forward to target
			originalPath := req.URL.Path
			trimmed := strings.TrimPrefix(originalPath, svc.Prefix)
			if trimmed == "" {
				trimmed = "/"
			}

			req.URL.Scheme = targetURL.Scheme
			req.URL.Host = targetURL.Host
			req.URL.Path = singleJoiningSlash(targetURL.Path, trimmed)

			// Set the Host header to the target host so downstream sees correct host
			req.Host = targetURL.Host

			if logger.IsDebugEnabled() &&
				req.Body != nil && req.Body != http.NoBody &&
				isLoggableContentType(req.Header.Get("Content-Type")) {
				if bodyBytes, restored, err := readBody(req.Body); err == nil {
					req.Body = restored
					req.ContentLength = int64(len(bodyBytes))
					logger.Debugf("[Request] [%s] [%s] | REQ BODY: %s",
						req.Method, originalPath, compactIfJSON(bodyBytes))
				}
			}
		},
		ModifyResponse: func(resp *http.Response) error {
			// Remove hop-by-hop headers from response
			removeHopHeaders(resp.Header)
			// Remove CORS headers from backend response — gateway owns CORS.
			// httputil.ReverseProxy uses Header.Add() when copying headers, so
			// any CORS headers from the backend would create duplicates alongside
			// the gateway's headers, causing browsers to reject the response.
			resp.Header.Del("Access-Control-Allow-Origin")
			resp.Header.Del("Access-Control-Allow-Methods")
			resp.Header.Del("Access-Control-Allow-Headers")
			resp.Header.Del("Access-Control-Allow-Credentials")
			resp.Header.Del("Access-Control-Max-Age")
			resp.Header.Del("Access-Control-Expose-Headers")

			if logger.IsDebugEnabled() &&
				resp.Body != nil &&
				isLoggableContentType(resp.Header.Get("Content-Type")) {
				if bodyBytes, restored, err := readBody(resp.Body); err == nil {
					resp.Body = restored
					resp.ContentLength = int64(len(bodyBytes))
					logger.Debugf("[Response] [%s] [%s] | RES [%d] BODY: %s",
						resp.Request.Method, resp.Request.URL.Path,
						resp.StatusCode, compactIfJSON(bodyBytes))
				}
			}

			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, req *http.Request, err error) {
			logger.Errorf("[Proxy] Error forwarding to %s: %v", svc.Name, err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"success":false,"message":"Service unavailable: ` + svc.Name + `"}`))
		},
	}

	return &ServiceProxy{
		Name:   svc.Name,
		Prefix: svc.Prefix,
		Target: targetURL,
		Proxy:  proxy,
	}, nil
}

// ServeHTTP forwards the request through the reverse proxy.
func (sp *ServiceProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	sp.Proxy.ServeHTTP(w, r)
}

// singleJoiningSlash joins two URL path segments with exactly one slash.
func singleJoiningSlash(a, b string) string {
	aSlash := strings.HasSuffix(a, "/")
	bSlash := strings.HasPrefix(b, "/")
	switch {
	case aSlash && bSlash:
		return a + b[1:]
	case !aSlash && !bSlash:
		return a + "/" + b
	}
	return a + b
}

// Hop-by-hop headers that should not be forwarded.
var hopHeaders = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Te",
	"Trailers",
	"Transfer-Encoding",
	"Upgrade",
}

func removeHopHeaders(h http.Header) {
	for _, hdr := range hopHeaders {
		h.Del(hdr)
	}
}
