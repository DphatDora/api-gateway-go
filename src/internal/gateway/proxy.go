package gateway

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"api-gateway/config"
	"api-gateway/package/logger"
)

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

			// Log the proxy action
			logger.Debugf("[Proxy] %s %s -> %s%s", req.Method, originalPath, targetURL.Host, req.URL.Path)
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
