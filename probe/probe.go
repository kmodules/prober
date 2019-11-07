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

package probe

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	api "kmodules.xyz/prober/api"
	api_v1 "kmodules.xyz/prober/api/v1"
	execprobe "kmodules.xyz/prober/probe/exec"
	httpprobe "kmodules.xyz/prober/probe/http"
	tcpprobe "kmodules.xyz/prober/probe/tcp"

	"github.com/appscode/go/log"
	core "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/rest"
)

type Prober struct {
	HttpGet  httpprobe.GetProber
	HttpPost httpprobe.PostProber
	Tcp      tcpprobe.Prober
	Exec     execprobe.Prober
	Config   *rest.Config
}

// NewProber creates a Prober instance that can be used to run httpGet, httpPost, tcp or exec probe.
func NewProber(config *rest.Config) *Prober {
	const followNonLocalRedirects = false

	return &Prober{
		HttpGet:  httpprobe.NewHttpGet(followNonLocalRedirects),
		HttpPost: httpprobe.NewHttpPost(followNonLocalRedirects),
		Tcp:      tcpprobe.New(),
		Exec:     execprobe.New(),
		Config:   config,
	}
}

func (pb *Prober) RunProbe(p *api_v1.Handler, pod *core.Pod, status core.PodStatus, container core.Container, timeout time.Duration) (api.Result, string, error) {
	if p.Exec != nil {
		log.Debug("Exec-Probe Pod: %v, Container: %v, Command: %v", pod, container, p.Exec.Command)
		return pb.Exec.Probe(pb.Config, pod, container, p.Exec.Command)
	}
	if p.HTTPGet != nil {
		scheme := strings.ToLower(string(p.HTTPGet.Scheme))
		host := p.HTTPGet.Host
		if host == "" {
			host = status.PodIP
		}
		port, err := extractPort(p.HTTPGet.Port, container)
		if err != nil {
			return api.Unknown, "", err
		}
		path := p.HTTPGet.Path
		log.Debug("HTTP-Probe Host: %v://%v, Port: %v, Path: %v", scheme, host, port, path)
		targetURL := formatURL(scheme, host, port, path)
		headers := buildHeader(p.HTTPGet.HTTPHeaders)
		log.Debug("HTTP-Probe Headers: %v", headers)
		return pb.HttpGet.Probe(targetURL, headers, timeout)
	}
	if p.HTTPPost != nil {
		scheme := strings.ToLower(string(p.HTTPPost.Scheme))
		host := p.HTTPPost.Host
		if host == "" {
			host = status.PodIP
		}
		port, err := extractPort(p.HTTPPost.Port, container)
		if err != nil {
			return api.Unknown, "", err
		}
		path := p.HTTPPost.Path
		log.Debug("HTTP-Probe Host: %v://%v, Port: %v, Path: %v", scheme, host, port, path)
		targetURL := formatURL(scheme, host, port, path)
		headers := buildHeader(p.HTTPPost.HTTPHeaders)
		log.Debug("HTTP-Probe Headers: %v", headers)
		return pb.HttpPost.Probe(targetURL, headers, p.HTTPPost.Form, p.HTTPPost.Body, timeout)
	}
	if p.TCPSocket != nil {
		port, err := extractPort(p.TCPSocket.Port, container)
		if err != nil {
			return api.Unknown, "", err
		}
		host := p.TCPSocket.Host
		if host == "" {
			host = status.PodIP
		}
		log.Debug("TCP-Probe Host: %v, Port: %v, Timeout: %v", host, port, timeout)
		return pb.Tcp.Probe(host, port, timeout)
	}
	log.Warningf("Failed to find probe builder for container: %v", container)
	return api.Unknown, "", fmt.Errorf("missing probe handler for %s:%s", formatPod(pod), container.Name)
}

// buildHeaderMap takes a list of HTTPHeader <name, value> string
// pairs and returns a populated string->[]string http.Header map.
func buildHeader(headerList []v1.HTTPHeader) http.Header {
	headers := make(http.Header)
	for _, header := range headerList {
		headers[header.Name] = append(headers[header.Name], header.Value)
	}
	return headers
}

func extractPort(param intstr.IntOrString, container core.Container) (int, error) {
	port := -1
	var err error
	switch param.Type {
	case intstr.Int:
		port = param.IntValue()
	case intstr.String:
		if port, err = findPortByName(container, param.StrVal); err != nil {
			// Last ditch effort - maybe it was an int stored as string?
			if port, err = strconv.Atoi(param.StrVal); err != nil {
				return port, err
			}
		}
	default:
		return port, fmt.Errorf("intOrString had no kind: %+v", param)
	}
	if port > 0 && port < 65536 {
		return port, nil
	}
	return port, fmt.Errorf("invalid port number: %v", port)
}

// findPortByName is a helper function to look up a port in a container by name.
func findPortByName(container core.Container, portName string) (int, error) {
	for _, port := range container.Ports {
		if port.Name == portName {
			return int(port.ContainerPort), nil
		}
	}
	return 0, fmt.Errorf("port %s not found", portName)
}

// formatURL formats a URL from args.  For testability.
func formatURL(scheme string, host string, port int, path string) *url.URL {
	u, err := url.Parse(path)
	// Something is busted with the path, but it's too late to reject it. Pass it along as is.
	if err != nil {
		u = &url.URL{
			Path: path,
		}
	}
	u.Scheme = scheme
	u.Host = net.JoinHostPort(host, strconv.Itoa(port))
	return u
}

// formatPod returns a string representing a pod in a consistent human readable format,
// with pod UID as part of the string.
func formatPod(pod *v1.Pod) string {
	return podDesc(pod.Name, pod.Namespace, pod.UID)
}

// podDesc returns a string representing a pod in a consistent human readable format,
// with pod UID as part of the string.
func podDesc(podName, podNamespace string, podUID types.UID) string {
	// Use underscore as the delimiter because it is not allowed in pod name
	// (DNS subdomain format), while allowed in the container name format.
	return fmt.Sprintf("%s_%s(%s)", podName, podNamespace, podUID)
}
