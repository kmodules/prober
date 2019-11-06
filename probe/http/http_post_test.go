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
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"kmodules.xyz/prober/probe"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

func TestHTTPPostProbeProxy(t *testing.T) {
	res := "welcome to http probe proxy"
	localProxy := "http://127.0.0.1:9098/"

	defer setEnv("http_proxy", localProxy)()
	defer setEnv("HTTP_PROXY", localProxy)()
	defer unsetEnv("no_proxy")()
	defer unsetEnv("NO_PROXY")()

	followNonLocalRedirects := true
	prober := NewHttpPost(followNonLocalRedirects)

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
	_, response, _ := prober.Probe(url, http.Header{}, nil, "", time.Second*3)

	if response == res {
		t.Errorf("proxy test unexpected error: the probe is using proxy")
	}
}

func TestHTTPPostProbeChecker(t *testing.T) {
	//handleReq := func(s int, body string) func(w http.ResponseWriter, r *http.Request) {
	//	return func(w http.ResponseWriter, r *http.Request) {
	//		w.WriteHeader(s)
	//		_, err := w.Write([]byte(body))
	//		utilruntime.Must(err)
	//	}
	//}
	handleReqWithForm := func(s int) func(w http.ResponseWriter, r *http.Request) {
		return func(w http.ResponseWriter, r *http.Request) {
			err := r.ParseForm()
			utilruntime.Must(err)

			if r.Header.Get(ContentType) != ContentUrlEncodedForm {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.WriteHeader(s)
			var formData []string
			for key, value := range r.Form {
				formData = append(formData, fmt.Sprintf("%s=%v", key, value))
			}

			sort.Strings(formData)
			_, err = w.Write([]byte(strings.Join(formData, "&")))
			utilruntime.Must(err)
		}
	}
	//handleReqWithBody := func(s int, body string) func(w http.ResponseWriter, r *http.Request) {
	//	return func(w http.ResponseWriter, r *http.Request) {
	//		w.WriteHeader(s)
	//		_, err := w.Write([]byte(body))
	//		utilruntime.Must(err)
	//	}
	//}
	//
	//// Echo handler that returns the contents of request headers in the body
	//headerEchoHandler := func(w http.ResponseWriter, r *http.Request) {
	//	w.WriteHeader(200)
	//	output := ""
	//	for k, arr := range r.Header {
	//		for _, v := range arr {
	//			output += fmt.Sprintf("%s: %s\n", k, v)
	//		}
	//	}
	//	_, err := w.Write([]byte(output))
	//	utilruntime.Must(err)
	//}
	//
	//redirectHandler := func(s int, bad bool) func(w http.ResponseWriter, r *http.Request) {
	//	return func(w http.ResponseWriter, r *http.Request) {
	//		if r.URL.Path == "/" {
	//			http.Redirect(w, r, "/new", s)
	//		} else if bad && r.URL.Path == "/new" {
	//			http.Error(w, "", http.StatusInternalServerError)
	//		}
	//	}
	//}

	followNonLocalRedirects := true
	prober := NewHttpPost(followNonLocalRedirects)
	testCases := []struct {
		handler    func(w http.ResponseWriter, r *http.Request)
		reqHeaders http.Header
		health     probe.Result
		accBody    string
		notBody    string
		form       *url.Values
		body       string
	}{
		// The probe will be filled in below.  This is primarily testing that an HTTP POST happens.
		{
			handler: handleReqWithForm(http.StatusOK),
			health:  probe.Success,
			form:    &url.Values{"name": {"form-test"}, "age": {"who-cares"}},
			accBody: "age=[who-cares]&name=[form-test]",
		},
		//{
		//	handler: headerEchoHandler,
		//	reqHeaders: http.Header{
		//		"X-Muffins-Or-Cupcakes": {"muffins"},
		//	},
		//	health:  probe.Success,
		//	accBody: "X-Muffins-Or-Cupcakes: muffins",
		//},
		////{
		////	handler: headerEchoHandler,
		////	reqHeaders: http.Header{
		////		"User-Agent": {"foo/1.0"},
		////	},
		////	health:  probe.Success,
		////	accBody: "User-Agent: foo/1.0",
		////},
		////{
		////	handler: headerEchoHandler,
		////	reqHeaders: http.Header{
		////		"User-Agent": {""},
		////	},
		////	health:  probe.Success,
		////	notBody: "User-Agent",
		////},
		////{
		////	handler:    headerEchoHandler,
		////	reqHeaders: http.Header{},
		////	health:     probe.Success,
		////	accBody:    "User-Agent: kmodules.xyz/client-go/release-11.0",
		////},
		////{
		////	// Echo handler that returns the contents of Host in the body
		////	handler: func(w http.ResponseWriter, r *http.Request) {
		////		w.WriteHeader(200)
		////		_, err := w.Write([]byte(r.Host))
		////		utilruntime.Must(err)
		////	},
		////	reqHeaders: http.Header{
		////		"Host": {"muffins.cupcakes.org"},
		////	},
		////	health:  probe.Success,
		////	accBody: "muffins.cupcakes.org",
		////},
		////{
		////	handler: handleReq(FailureCode, "fail body"),
		////	health:  probe.Failure,
		////},
		////{
		////	handler: handleReq(http.StatusInternalServerError, "fail body"),
		////	health:  probe.Failure,
		////},
		////{
		////	handler: func(w http.ResponseWriter, r *http.Request) {
		////		time.Sleep(3 * time.Second)
		////	},
		////	health: probe.Failure,
		////},
		////{
		////	handler: redirectHandler(http.StatusMovedPermanently, false), // 301
		////	health:  probe.Success,
		////},
		////{
		////	handler: redirectHandler(http.StatusMovedPermanently, true), // 301
		////	health:  probe.Failure,
		////},
		////{
		////	handler: redirectHandler(http.StatusFound, false), // 302
		////	health:  probe.Success,
		////},
		////{
		////	handler: redirectHandler(http.StatusFound, true), // 302
		////	health:  probe.Failure,
		////},
		////{
		////	handler: redirectHandler(http.StatusTemporaryRedirect, false), // 307
		////	health:  probe.Success,
		////},
		////{
		////	handler: redirectHandler(http.StatusTemporaryRedirect, true), // 307
		////	health:  probe.Failure,
		////},
		////{
		////	handler: redirectHandler(http.StatusPermanentRedirect, false), // 308
		////	health:  probe.Success,
		////},
		////{
		////	handler: redirectHandler(http.StatusPermanentRedirect, true), // 308
		////	health:  probe.Failure,
		////},
	}
	for i, test := range testCases {
		t.Run(fmt.Sprintf("case-%2d", i), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				test.handler(w, r)
			}))
			defer server.Close()
			u, err := url.Parse(server.URL)
			if err != nil {
				t.Errorf("case %d: unexpected error: %v", i, err)
			}
			_, port, err := net.SplitHostPort(u.Host)
			if err != nil {
				t.Errorf("case %d: unexpected error: %v", i, err)
			}
			_, err = strconv.Atoi(port)
			if err != nil {
				t.Errorf("case %d: unexpected error: %v", i, err)
			}
			health, output, err := prober.Probe(u, test.reqHeaders, test.form, test.body, 1*time.Second)
			if test.health == probe.Unknown && err == nil {
				t.Errorf("case %d: expected error", i)
			}
			if test.health != probe.Unknown && err != nil {
				t.Errorf("case %d: unexpected error: %v", i, err)
			}
			if health != test.health {
				t.Errorf("case %d: expected %v, got %v", i, test.health, health)
			}
			if health != probe.Failure && test.health != probe.Failure {
				if !strings.Contains(output, test.accBody) {
					t.Errorf("Expected response body to contain %v, got %v", test.accBody, output)
				}
				if test.notBody != "" && strings.Contains(output, test.notBody) {
					t.Errorf("Expected response not to contain %v, got %v", test.notBody, output)
				}
			}
		})
	}
}

//func TestHTTPProbeChecker_NonLocalRedirects(t *testing.T) {
//	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//		switch r.URL.Path {
//		case "/redirect":
//			loc, _ := url.QueryUnescape(r.URL.Query().Get("loc"))
//			http.Redirect(w, r, loc, http.StatusFound)
//		case "/loop":
//			http.Redirect(w, r, "/loop", http.StatusFound)
//		case "/success":
//			w.WriteHeader(http.StatusOK)
//		default:
//			http.Error(w, "", http.StatusInternalServerError)
//		}
//	})
//	server := httptest.NewServer(handler)
//	defer server.Close()
//
//	newportServer := httptest.NewServer(handler)
//	defer newportServer.Close()
//
//	testCases := map[string]struct {
//		redirect             string
//		expectLocalResult    probe.Result
//		expectNonLocalResult probe.Result
//	}{
//		"local success":   {"/success", probe.Success, probe.Success},
//		"local fail":      {"/fail", probe.Failure, probe.Failure},
//		"newport success": {newportServer.URL + "/success", probe.Success, probe.Success},
//		"newport fail":    {newportServer.URL + "/fail", probe.Failure, probe.Failure},
//		"bogus nonlocal":  {"http://0.0.0.0/fail", probe.Warning, probe.Failure},
//		"redirect loop":   {"/loop", probe.Failure, probe.Failure},
//	}
//	for desc, test := range testCases {
//		t.Run(desc+"-local", func(t *testing.T) {
//			followNonLocalRedirects := false
//			prober := NewHttpGet(followNonLocalRedirects)
//			target, err := url.Parse(server.URL + "/redirect?loc=" + url.QueryEscape(test.redirect))
//			require.NoError(t, err)
//			result, _, _ := prober.Probe(target, nil, wait.ForeverTestTimeout)
//			assert.Equal(t, test.expectLocalResult, result)
//		})
//		t.Run(desc+"-nonlocal", func(t *testing.T) {
//			followNonLocalRedirects := true
//			prober := NewHttpGet(followNonLocalRedirects)
//			target, err := url.Parse(server.URL + "/redirect?loc=" + url.QueryEscape(test.redirect))
//			require.NoError(t, err)
//			result, _, _ := prober.Probe(target, nil, wait.ForeverTestTimeout)
//			assert.Equal(t, test.expectNonLocalResult, result)
//		})
//	}
//}
//
//func TestHTTPProbeChecker_HostHeaderPreservedAfterRedirect(t *testing.T) {
//	successHostHeader := "www.success.com"
//	failHostHeader := "www.fail.com"
//
//	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//		switch r.URL.Path {
//		case "/redirect":
//			http.Redirect(w, r, "/success", http.StatusFound)
//		case "/success":
//			if r.Host == successHostHeader {
//				w.WriteHeader(http.StatusOK)
//			} else {
//				http.Error(w, "", http.StatusBadRequest)
//			}
//		default:
//			http.Error(w, "", http.StatusInternalServerError)
//		}
//	})
//	server := httptest.NewServer(handler)
//	defer server.Close()
//
//	testCases := map[string]struct {
//		hostHeader     string
//		expectedResult probe.Result
//	}{
//		"success": {successHostHeader, probe.Success},
//		"fail":    {failHostHeader, probe.Failure},
//	}
//	for desc, test := range testCases {
//		headers := http.Header{}
//		headers.Add("Host", test.hostHeader)
//		t.Run(desc+"local", func(t *testing.T) {
//			followNonLocalRedirects := false
//			prober := NewHttpGet(followNonLocalRedirects)
//			target, err := url.Parse(server.URL + "/redirect")
//			require.NoError(t, err)
//			result, _, _ := prober.Probe(target, headers, wait.ForeverTestTimeout)
//			assert.Equal(t, test.expectedResult, result)
//		})
//		t.Run(desc+"nonlocal", func(t *testing.T) {
//			followNonLocalRedirects := true
//			prober := NewHttpGet(followNonLocalRedirects)
//			target, err := url.Parse(server.URL + "/redirect")
//			require.NoError(t, err)
//			result, _, _ := prober.Probe(target, headers, wait.ForeverTestTimeout)
//			assert.Equal(t, test.expectedResult, result)
//		})
//	}
//}
//
//func TestHTTPProbeChecker_PayloadTruncated(t *testing.T) {
//	successHostHeader := "www.success.com"
//	oversizePayload := bytes.Repeat([]byte("a"), maxRespBodyLength+1)
//	truncatedPayload := bytes.Repeat([]byte("a"), maxRespBodyLength)
//
//	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//		switch r.URL.Path {
//		case "/success":
//			if r.Host == successHostHeader {
//				w.WriteHeader(http.StatusOK)
//				_, err := w.Write(oversizePayload)
//				utilruntime.Must(err)
//			} else {
//				http.Error(w, "", http.StatusBadRequest)
//			}
//		default:
//			http.Error(w, "", http.StatusInternalServerError)
//		}
//	})
//	server := httptest.NewServer(handler)
//	defer server.Close()
//
//	headers := http.Header{}
//	headers.Add("Host", successHostHeader)
//	t.Run("truncated payload", func(t *testing.T) {
//		prober := NewHttpGet(false)
//		target, err := url.Parse(server.URL + "/success")
//		require.NoError(t, err)
//		result, body, err := prober.Probe(target, headers, wait.ForeverTestTimeout)
//		assert.NoError(t, err)
//		assert.Equal(t, result, probe.Success)
//		assert.Equal(t, body, string(truncatedPayload))
//	})
//}
//
//func TestHTTPProbeChecker_PayloadNormal(t *testing.T) {
//	successHostHeader := "www.success.com"
//	normalPayload := bytes.Repeat([]byte("a"), maxRespBodyLength-1)
//
//	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//		switch r.URL.Path {
//		case "/success":
//			if r.Host == successHostHeader {
//				w.WriteHeader(http.StatusOK)
//				_, err := w.Write(normalPayload)
//				utilruntime.Must(err)
//			} else {
//				http.Error(w, "", http.StatusBadRequest)
//			}
//		default:
//			http.Error(w, "", http.StatusInternalServerError)
//		}
//	})
//	server := httptest.NewServer(handler)
//	defer server.Close()
//
//	headers := http.Header{}
//	headers.Add("Host", successHostHeader)
//	t.Run("normal payload", func(t *testing.T) {
//		prober := NewHttpGet(false)
//		target, err := url.Parse(server.URL + "/success")
//		require.NoError(t, err)
//		result, body, err := prober.Probe(target, headers, wait.ForeverTestTimeout)
//		assert.NoError(t, err)
//		assert.Equal(t, result, probe.Success)
//		assert.Equal(t, body, string(normalPayload))
//	})
//}
