package probe

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"kmodules.xyz/prober/api"
	prober_v1 "kmodules.xyz/prober/api/v1"

	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestFormatURL(t *testing.T) {
	testCases := []struct {
		scheme string
		host   string
		port   int
		path   string
		result string
	}{
		{"http", "localhost", 93, "", "http://localhost:93"},
		{"https", "localhost", 93, "/path", "https://localhost:93/path"},
		{"http", "localhost", 93, "?foo", "http://localhost:93?foo"},
		{"https", "localhost", 93, "/path?bar", "https://localhost:93/path?bar"},
	}
	for _, test := range testCases {
		url := formatURL(test.scheme, test.host, test.port, test.path)
		if url.String() != test.result {
			t.Errorf("Expected %s, got %s", test.result, url.String())
		}
	}
}

func TestFindPortByName(t *testing.T) {
	container := core.Container{
		Ports: []core.ContainerPort{
			{
				Name:          "foo",
				ContainerPort: 8080,
			},
			{
				Name:          "bar",
				ContainerPort: 9000,
			},
		},
	}
	want := 8080
	got, err := findPortByName(container, "foo")
	if got != want || err != nil {
		t.Errorf("Expected %v, got %v, err: %v", want, got, err)
	}
}

func TestExtractPort(t *testing.T) {
	pod := &core.Pod{
		Spec: core.PodSpec{
			Containers: []core.Container{
				{
					Name: "foo",
					Ports: []core.ContainerPort{
						{
							Name:          "foo-port",
							ContainerPort: 8080,
						},
					},
				},
				{
					Name: "bar",
					Ports: []core.ContainerPort{
						{
							Name:          "bar-port",
							ContainerPort: 9090,
						},
					},
				},
				{
					Name: "fizz",
					Ports: []core.ContainerPort{
						{
							Name:          "fizz-port",
							ContainerPort: 65538,
						},
					},
				},
			},
		},
	}
	testCases := []struct {
		name           string
		param          intstr.IntOrString
		pod            *core.Pod
		containerName  string
		expectedPort   int
		expectedErrMsg string
	}{
		{name: "Find port by IntValue", param: intstr.FromInt(8080), pod: pod, containerName: "foo", expectedPort: 8080, expectedErrMsg: ""},
		{name: "Find port by Name", param: intstr.FromString("foo-port"), pod: pod, containerName: "foo", expectedPort: 8080, expectedErrMsg: ""},
		{name: "Invalid Pod", param: intstr.FromInt(8080), pod: nil, containerName: "foo", expectedPort: -1, expectedErrMsg: "failed to extract port. invalid pod"},
		{name: "Unknown Container", param: intstr.FromInt(8080), pod: pod, containerName: "buzz", expectedPort: -1, expectedErrMsg: "failed to extract port. container not found"},
		{name: "Invalid Port", param: intstr.FromString("fizz-port"), pod: pod, containerName: "fizz", expectedPort: 65538, expectedErrMsg: "invalid port number: 65538"},
	}

	for i, test := range testCases {
		t.Run(fmt.Sprintf("Case %d: %s", i, test.name), func(t *testing.T) {
			port, err := extractPort(test.param, test.pod, test.containerName)
			if err != nil {
				if err.Error() != test.expectedErrMsg {
					t.Errorf("Expected Error Mesage: %v, Found: %v", test.expectedErrMsg, err.Error())
				}
			}
			if port != test.expectedPort {
				t.Errorf("Expected port: %v, Found: %v", test.expectedPort, port)
			}
		})
	}
}

func TestRunProbe(t *testing.T) {
	genericHandler := func(responseCode int) func(w http.ResponseWriter, r *http.Request) {
		return func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(responseCode)
		}
	}
	pod := &core.Pod{
		Spec: core.PodSpec{
			Containers: []core.Container{
				{
					Name: "foo",
					Ports: []core.ContainerPort{
						{
							Name:          "foo-port",
							ContainerPort: 8920,
						},
					},
				},
			},
		},
		Status: core.PodStatus{PodIP: "127.0.0.1"},
	}
	testCases := []struct {
		name           string
		probe          *prober_v1.Handler
		handler        func(w http.ResponseWriter, r *http.Request)
		pod            *core.Pod
		containerName  string
		expectedResult api.Result
		expectedErrMsg string
	}{
		//==================== HTTP Get Probe ======================
		{
			name: "HTTPGet: host and port specified (success check)",
			probe: &prober_v1.Handler{
				HTTPGet: &core.HTTPGetAction{
					Scheme: "HTTP",
					Host:   "127.0.0.1",
					Path:   "/success",
					Port:   intstr.FromInt(8920),
				},
			},
			handler:        genericHandler(http.StatusOK),
			pod:            pod,
			containerName:  "foo",
			expectedResult: api.Success,
			expectedErrMsg: "",
		},
		{
			name: "HTTPGet: host and port specified (failure check)",
			probe: &prober_v1.Handler{
				HTTPGet: &core.HTTPGetAction{
					Scheme: "HTTP",
					Host:   "127.0.0.1",
					Path:   "/fail",
					Port:   intstr.FromInt(8920),
				},
			},
			handler:        genericHandler(http.StatusBadRequest),
			pod:            pod,
			containerName:  "foo",
			expectedResult: api.Failure,
			expectedErrMsg: "",
		},
		{
			name: "HTTPGet: host and port from pod (success check)",
			probe: &prober_v1.Handler{
				HTTPGet: &core.HTTPGetAction{
					Scheme: "HTTP",
					Path:   "/success",
					Port:   intstr.FromString("foo-port"),
				},
			},
			handler:        genericHandler(http.StatusOK),
			pod:            pod,
			containerName:  "foo",
			expectedResult: api.Success,
			expectedErrMsg: "",
		},
		{
			name: "HTTPGet: host and port from pod (failure check)",
			probe: &prober_v1.Handler{
				HTTPGet: &core.HTTPGetAction{
					Scheme: "HTTP",
					Path:   "/fail",
					Port:   intstr.FromString("foo-port"),
				},
			},
			handler:        genericHandler(http.StatusBadRequest),
			pod:            pod,
			containerName:  "foo",
			expectedResult: api.Failure,
			expectedErrMsg: "",
		},
		{
			name: "HTTPGet: invalid pod",
			probe: &prober_v1.Handler{
				HTTPGet: &core.HTTPGetAction{
					Scheme: "HTTP",
					Host:   "127.0.0.1",
					Path:   "/success",
					Port:   intstr.FromString("foo-port"),
				},
			},
			handler:        genericHandler(http.StatusOK),
			pod:            nil,
			containerName:  "foo",
			expectedResult: api.Unknown,
			expectedErrMsg: "failed to extract port. invalid pod",
		},
		{
			name: "HTTPGet: unknown container",
			probe: &prober_v1.Handler{
				HTTPGet: &core.HTTPGetAction{
					Scheme: "HTTP",
					Path:   "/fail",
					Port:   intstr.FromString("bar-port"),
				},
			},
			handler:        genericHandler(http.StatusOK),
			pod:            pod,
			containerName:  "bar",
			expectedResult: api.Unknown,
			expectedErrMsg: "failed to extract port. container not found",
		},
		//========================== HTTP Post Probe======================
		{
			name: "HTTPPost: host and port specified (success check)",
			probe: &prober_v1.Handler{
				HTTPPost: &prober_v1.HTTPPostAction{
					Scheme: "HTTP",
					Host:   "127.0.0.1",
					Path:   "/success",
					Port:   intstr.FromInt(8920),
				},
			},
			handler:        genericHandler(http.StatusOK),
			pod:            pod,
			containerName:  "foo",
			expectedResult: api.Success,
			expectedErrMsg: "",
		},
		{
			name: "HTTPPost: host and port specified (failure check)",
			probe: &prober_v1.Handler{
				HTTPPost: &prober_v1.HTTPPostAction{
					Scheme: "HTTP",
					Host:   "127.0.0.1",
					Path:   "/fail",
					Port:   intstr.FromInt(8920),
				},
			},
			handler:        genericHandler(http.StatusBadRequest),
			pod:            pod,
			containerName:  "foo",
			expectedResult: api.Failure,
			expectedErrMsg: "",
		},
		{
			name: "HTTPPost: host and port from pod (success check)",
			probe: &prober_v1.Handler{
				HTTPPost: &prober_v1.HTTPPostAction{
					Scheme: "HTTP",
					Path:   "/success",
					Port:   intstr.FromString("foo-port"),
				},
			},
			handler:        genericHandler(http.StatusOK),
			pod:            pod,
			containerName:  "foo",
			expectedResult: api.Success,
			expectedErrMsg: "",
		},
		{
			name: "HTTPPost: host and port from pod (failure check)",
			probe: &prober_v1.Handler{
				HTTPPost: &prober_v1.HTTPPostAction{
					Scheme: "HTTP",
					Path:   "/fail",
					Port:   intstr.FromString("foo-port"),
				},
			},
			handler:        genericHandler(http.StatusBadRequest),
			pod:            pod,
			containerName:  "foo",
			expectedResult: api.Failure,
			expectedErrMsg: "",
		},
		{
			name: "HTTPPost: invalid pod",
			probe: &prober_v1.Handler{
				HTTPPost: &prober_v1.HTTPPostAction{
					Scheme: "HTTP",
					Host:   "127.0.0.1",
					Path:   "/success",
					Port:   intstr.FromString("foo-port"),
				},
			},
			handler:        genericHandler(http.StatusOK),
			pod:            nil,
			containerName:  "foo",
			expectedResult: api.Unknown,
			expectedErrMsg: "failed to extract port. invalid pod",
		},
		{
			name: "HTTPPost: unknown container",
			probe: &prober_v1.Handler{
				HTTPPost: &prober_v1.HTTPPostAction{
					Scheme: "HTTP",
					Path:   "/fail",
					Port:   intstr.FromString("bar-port"),
				},
			},
			handler:        genericHandler(http.StatusOK),
			pod:            pod,
			containerName:  "bar",
			expectedResult: api.Unknown,
			expectedErrMsg: "failed to extract port. container not found",
		},
		//======================= TCP Probe ====================
		{
			name: "TCP: host and port specified (success check)",
			probe: &prober_v1.Handler{
				TCPSocket: &core.TCPSocketAction{
					Host: "127.0.0.1",
					Port: intstr.FromInt(8920),
				},
			},
			handler:        genericHandler(http.StatusOK),
			pod:            pod,
			containerName:  "foo",
			expectedResult: api.Success,
			expectedErrMsg: "",
		},
		{
			name: "TCP: host and port specified (failure check)",
			probe: &prober_v1.Handler{
				TCPSocket: &core.TCPSocketAction{
					Host: "127.0.0.1",
					Port: intstr.FromInt(8899),
				},
			},
			handler:        genericHandler(http.StatusBadRequest),
			pod:            pod,
			containerName:  "foo",
			expectedResult: api.Failure,
			expectedErrMsg: "",
		},
		{
			name: "TCP: host and port from pod (success check)",
			probe: &prober_v1.Handler{
				TCPSocket: &core.TCPSocketAction{
					Port: intstr.FromString("foo-port"),
				},
			},
			handler:        genericHandler(http.StatusOK),
			pod:            pod,
			containerName:  "foo",
			expectedResult: api.Success,
			expectedErrMsg: "",
		},
		{
			name: "TCP: host and port from pod (failure check)",
			probe: &prober_v1.Handler{
				TCPSocket: &core.TCPSocketAction{
					Port: intstr.FromString("foo-port"),
				},
			},
			handler: genericHandler(http.StatusBadRequest),
			pod: &core.Pod{
				Spec: core.PodSpec{
					Containers: []core.Container{
						{
							Name: "foo",
							Ports: []core.ContainerPort{
								{
									Name:          "foo-port",
									ContainerPort: 8899,
								},
							},
						},
					},
				},
				Status: pod.Status,
			},
			containerName:  "foo",
			expectedResult: api.Failure,
			expectedErrMsg: "",
		},
		{
			name: "TCP: invalid pod",
			probe: &prober_v1.Handler{
				TCPSocket: &core.TCPSocketAction{
					Host: "127.0.0.1",
					Port: intstr.FromString("foo-port"),
				},
			},
			handler:        genericHandler(http.StatusOK),
			pod:            nil,
			containerName:  "foo",
			expectedResult: api.Unknown,
			expectedErrMsg: "failed to extract port. invalid pod",
		},
		{
			name: "TCP: unknown container",
			probe: &prober_v1.Handler{
				TCPSocket: &core.TCPSocketAction{
					Port: intstr.FromString("bar-port"),
				},
			},
			handler:        genericHandler(http.StatusOK),
			pod:            pod,
			containerName:  "bar",
			expectedResult: api.Unknown,
			expectedErrMsg: "failed to extract port. container not found",
		},
	}
	prober := NewProber(nil)
	for i, test := range testCases {
		t.Run(fmt.Sprintf("Case %d: %s", i, test.name), func(t *testing.T) {
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				test.handler(w, r)
			}))
			customListener, err := net.Listen("tcp", "127.0.0.1:8920")
			if err != nil {
				t.Errorf("failed to create custom listener")
			}
			server.Listener.Close()
			server.Listener = customListener
			server.Start()
			defer server.Close()

			result, response, err := prober.RunProbe(test.probe, test.pod, test.containerName, time.Second*30)
			if err != nil {
				if err.Error() != test.expectedErrMsg {
					t.Errorf("Expected error message: %v, Found: %v", test.expectedErrMsg, err.Error())
				}
			}
			if result != test.expectedResult {
				t.Errorf("Expect result: %v, Found: %v. Respone: %v", test.expectedResult, result, response)
			}
		})
	}
}
