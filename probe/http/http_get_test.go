/*
Copyright 2015 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

//nolint:goconst
package http

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	api "kmodules.xyz/prober/api"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
)

// ref: https://github.com/golang/go/blob/release-branch.go1.14/src/net/http/server.go#L1079-L1094
const FailureCode int = 599

func setEnv(key, value string) func() {
	originalValue := os.Getenv(key)
	utilruntime.Must(os.Setenv(key, value))
	if len(originalValue) > 0 {
		return func() {
			utilruntime.Must(os.Setenv(key, originalValue))
		}
	}
	return func() {}
}

func unsetEnv(key string) func() {
	originalValue := os.Getenv(key)
	utilruntime.Must(os.Unsetenv(key))
	if len(originalValue) > 0 {
		return func() {
			utilruntime.Must(os.Setenv(key, originalValue))
		}
	}
	return func() {}
}

func TestHTTPGetProbeProxy(t *testing.T) {
	res := "welcome to http probe proxy"
	localProxy := "http://127.0.0.1:9098/"

	defer setEnv("http_proxy", localProxy)()
	defer setEnv("HTTP_PROXY", localProxy)()
	defer unsetEnv("no_proxy")()
	defer unsetEnv("NO_PROXY")()

	followNonLocalRedirects := true
	prober := NewHttpGet(followNonLocalRedirects)

	go func() {
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			_, err := fmt.Fprint(w, res)
			utilruntime.Must(err)
		})
		err := http.ListenAndServe(":9098", nil)
		if err != nil {
			t.Errorf("Failed to start foo server: localhost:9098")
		}
	}()

	// take some time to wait server boot
	time.Sleep(2 * time.Second)
	url, err := url.Parse("http://example.com")
	if err != nil {
		t.Errorf("proxy test unexpected error: %v", err)
	}
	_, response, _ := prober.Probe(url, http.Header{}, time.Second*3)

	if response == res {
		t.Errorf("proxy test unexpected error: the probe is using proxy")
	}
}

func TestHTTPProbeGetChecker(t *testing.T) {
	handleReq := func(code int, body string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(code)
			_, err := w.Write([]byte(body))
			utilruntime.Must(err)
		}
	}

	// Echo handler that returns the contents of request headers in the body
	headerEchoHandler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		output := ""
		for k, arr := range r.Header {
			for _, v := range arr {
				output += fmt.Sprintf("%s: %s\n", k, v)
			}
		}
		_, err := w.Write([]byte(output))
		utilruntime.Must(err)
	}

	redirectHandler := func(s int, bad bool) func(w http.ResponseWriter, r *http.Request) {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/" {
				http.Redirect(w, r, "/new", s)
			} else if bad && r.URL.Path == "/new" {
				http.Error(w, "", http.StatusInternalServerError)
			}
		}
	}

	followNonLocalRedirects := true
	prober := NewHttpGet(followNonLocalRedirects)
	testCases := []struct {
		handler    http.HandlerFunc
		reqHeaders http.Header
		health     api.Result
		accBody    string
		notBody    string
	}{
		// The probe will be filled in below.  This is primarily testing that an HTTP GET happens.
		{
			handler: handleReq(http.StatusOK, "ok body"),
			health:  api.Success,
			accBody: "ok body",
		},
		{
			handler: headerEchoHandler,
			reqHeaders: http.Header{
				"X-Muffins-Or-Cupcakes": {"muffins"},
			},
			health:  api.Success,
			accBody: "X-Muffins-Or-Cupcakes: muffins",
		},
		{
			handler: headerEchoHandler,
			reqHeaders: http.Header{
				"User-Agent": {"foo/1.0"},
			},
			health:  api.Success,
			accBody: "User-Agent: foo/1.0",
		},
		{
			handler: headerEchoHandler,
			reqHeaders: http.Header{
				"User-Agent": {""},
			},
			health:  api.Success,
			notBody: "User-Agent",
		},
		{
			handler:    headerEchoHandler,
			reqHeaders: http.Header{},
			health:     api.Success,
			accBody:    "User-Agent: kmodules.xyz/client-go/release-11.0",
		},
		{
			// Echo handler that returns the contents of Host in the body
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(200)
				_, err := w.Write([]byte(r.Host))
				utilruntime.Must(err)
			},
			reqHeaders: http.Header{
				"Host": {"muffins.cupcakes.org"},
			},
			health:  api.Success,
			accBody: "muffins.cupcakes.org",
		},
		{
			handler: handleReq(FailureCode, "fail body"),
			health:  api.Failure,
		},
		{
			handler: handleReq(http.StatusInternalServerError, "fail body"),
			health:  api.Failure,
		},
		{
			handler: func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(3 * time.Second)
			},
			health: api.Failure,
		},
		{
			handler: redirectHandler(http.StatusMovedPermanently, false), // 301
			health:  api.Success,
		},
		{
			handler: redirectHandler(http.StatusMovedPermanently, true), // 301
			health:  api.Failure,
		},
		{
			handler: redirectHandler(http.StatusFound, false), // 302
			health:  api.Success,
		},
		{
			handler: redirectHandler(http.StatusFound, true), // 302
			health:  api.Failure,
		},
		{
			handler: redirectHandler(http.StatusTemporaryRedirect, false), // 307
			health:  api.Success,
		},
		{
			handler: redirectHandler(http.StatusTemporaryRedirect, true), // 307
			health:  api.Failure,
		},
		{
			handler: redirectHandler(http.StatusPermanentRedirect, false), // 308
			health:  api.Success,
		},
		{
			handler: redirectHandler(http.StatusPermanentRedirect, true), // 308
			health:  api.Failure,
		},
	}
	for idx := range testCases {
		tt := testCases[idx]
		t.Run(fmt.Sprintf("case-%2d", idx), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				tt.handler(w, r)
			}))
			defer server.Close() // nolint:errcheck
			u, err := url.Parse(server.URL)
			if err != nil {
				t.Errorf("case %d: unexpected error: %v", idx, err)
			}
			_, port, err := net.SplitHostPort(u.Host)
			if err != nil {
				t.Errorf("case %d: unexpected error: %v", idx, err)
			}
			_, err = strconv.Atoi(port)
			if err != nil {
				t.Errorf("case %d: unexpected error: %v", idx, err)
			}
			health, output, err := prober.Probe(u, tt.reqHeaders, 1*time.Second)
			if tt.health == api.Unknown && err == nil {
				t.Errorf("case %d: expected error", idx)
			}
			if tt.health != api.Unknown && err != nil {
				t.Errorf("case %d: unexpected error: %v", idx, err)
			}
			if health != tt.health {
				t.Errorf("case %d: expected %v, got %v", idx, tt.health, health)
			}
			if health != api.Failure && tt.health != api.Failure {
				if !strings.Contains(output, tt.accBody) {
					t.Errorf("Expected response body to contain %v, got %v", tt.accBody, output)
				}
				if tt.notBody != "" && strings.Contains(output, tt.notBody) {
					t.Errorf("Expected response not to contain %v, got %v", tt.notBody, output)
				}
			}
		})
	}
}

func TestHTTPProbeChecker_NonLocalRedirects(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redirect":
			loc, _ := url.QueryUnescape(r.URL.Query().Get("loc"))
			http.Redirect(w, r, loc, http.StatusFound)
		case "/loop":
			http.Redirect(w, r, "/loop", http.StatusFound)
		case "/success":
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, "", http.StatusInternalServerError)
		}
	})
	server := httptest.NewServer(handler)
	defer server.Close() // nolint:errcheck

	newportServer := httptest.NewServer(handler)
	defer newportServer.Close() // nolint:errcheck

	testCases := map[string]struct {
		redirect             string
		expectLocalResult    api.Result
		expectNonLocalResult api.Result
	}{
		"local success":   {"/success", api.Success, api.Success},
		"local fail":      {"/fail", api.Failure, api.Failure},
		"newport success": {newportServer.URL + "/success", api.Success, api.Success},
		"newport fail":    {newportServer.URL + "/fail", api.Failure, api.Failure},
		"bogus nonlocal":  {"http://0.0.0.0/fail", api.Warning, api.Failure},
		"redirect loop":   {"/loop", api.Failure, api.Failure},
	}
	for idx := range testCases {
		tt := testCases[idx]
		t.Run(idx+"-local", func(t *testing.T) {
			followNonLocalRedirects := false
			prober := NewHttpGet(followNonLocalRedirects)
			target, err := url.Parse(server.URL + "/redirect?loc=" + url.QueryEscape(tt.redirect))
			require.NoError(t, err)
			result, _, _ := prober.Probe(target, nil, wait.ForeverTestTimeout)
			assert.Equal(t, tt.expectLocalResult, result)
		})
		t.Run(idx+"-nonlocal", func(t *testing.T) {
			followNonLocalRedirects := true
			prober := NewHttpGet(followNonLocalRedirects)
			target, err := url.Parse(server.URL + "/redirect?loc=" + url.QueryEscape(tt.redirect))
			require.NoError(t, err)
			result, _, _ := prober.Probe(target, nil, wait.ForeverTestTimeout)
			assert.Equal(t, tt.expectNonLocalResult, result)
		})
	}
}

func TestHTTPProbeChecker_HostHeaderPreservedAfterRedirect(t *testing.T) {
	successHostHeader := "www.success.com"
	failHostHeader := "www.fail.com"

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redirect":
			http.Redirect(w, r, "/success", http.StatusFound)
		case "/success":
			if r.Host == successHostHeader {
				w.WriteHeader(http.StatusOK)
			} else {
				http.Error(w, "", http.StatusBadRequest)
			}
		default:
			http.Error(w, "", http.StatusInternalServerError)
		}
	})
	server := httptest.NewServer(handler)
	defer server.Close() // nolint:errcheck

	testCases := map[string]struct {
		hostHeader     string
		expectedResult api.Result
	}{
		"success": {successHostHeader, api.Success},
		"fail":    {failHostHeader, api.Failure},
	}
	for idx := range testCases {
		tt := testCases[idx]
		headers := http.Header{}
		headers.Add("Host", tt.hostHeader)
		t.Run(idx+"local", func(t *testing.T) {
			followNonLocalRedirects := false
			prober := NewHttpGet(followNonLocalRedirects)
			target, err := url.Parse(server.URL + "/redirect")
			require.NoError(t, err)
			result, _, _ := prober.Probe(target, headers, wait.ForeverTestTimeout)
			assert.Equal(t, tt.expectedResult, result)
		})
		t.Run(idx+"nonlocal", func(t *testing.T) {
			followNonLocalRedirects := true
			prober := NewHttpGet(followNonLocalRedirects)
			target, err := url.Parse(server.URL + "/redirect")
			require.NoError(t, err)
			result, _, _ := prober.Probe(target, headers, wait.ForeverTestTimeout)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestHTTPProbeChecker_PayloadTruncated(t *testing.T) {
	successHostHeader := "www.success.com"
	oversizePayload := bytes.Repeat([]byte("a"), maxRespBodyLength+1)
	truncatedPayload := bytes.Repeat([]byte("a"), maxRespBodyLength)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/success":
			if r.Host == successHostHeader {
				w.WriteHeader(http.StatusOK)
				_, err := w.Write(oversizePayload)
				utilruntime.Must(err)
			} else {
				http.Error(w, "", http.StatusBadRequest)
			}
		default:
			http.Error(w, "", http.StatusInternalServerError)
		}
	})
	server := httptest.NewServer(handler)
	defer server.Close() // nolint:errcheck

	headers := http.Header{}
	headers.Add("Host", successHostHeader)
	t.Run("truncated payload", func(t *testing.T) {
		prober := NewHttpGet(false)
		target, err := url.Parse(server.URL + "/success")
		require.NoError(t, err)
		result, body, err := prober.Probe(target, headers, wait.ForeverTestTimeout)
		assert.NoError(t, err)
		assert.Equal(t, result, api.Success)
		assert.Equal(t, body, string(truncatedPayload))
	})
}

func TestHTTPProbeChecker_PayloadNormal(t *testing.T) {
	successHostHeader := "www.success.com"
	normalPayload := bytes.Repeat([]byte("a"), maxRespBodyLength-1)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/success":
			if r.Host == successHostHeader {
				w.WriteHeader(http.StatusOK)
				_, err := w.Write(normalPayload)
				utilruntime.Must(err)
			} else {
				http.Error(w, "", http.StatusBadRequest)
			}
		default:
			http.Error(w, "", http.StatusInternalServerError)
		}
	})
	server := httptest.NewServer(handler)
	defer server.Close() // nolint:errcheck

	headers := http.Header{}
	headers.Add("Host", successHostHeader)
	t.Run("normal payload", func(t *testing.T) {
		prober := NewHttpGet(false)
		target, err := url.Parse(server.URL + "/success")
		require.NoError(t, err)
		result, body, err := prober.Probe(target, headers, wait.ForeverTestTimeout)
		assert.NoError(t, err)
		assert.Equal(t, result, api.Success)
		assert.Equal(t, body, string(normalPayload))
	})
}
