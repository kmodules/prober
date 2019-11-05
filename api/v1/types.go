package v1

import (
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// Handler defines a specific action that should be taken
// TODO: pass structured data to these actions, and document that data here.
type Handler struct {
	// One and only one of the following should be specified.
	// Exec specifies the action to take.
	// +optional
	Exec *core.ExecAction `json:"exec,omitempty"`
	// HTTPGet specifies the http Get request to perform.
	// +optional
	HTTPGet *core.HTTPGetAction `json:"httpGet,omitempty"`
	// HTTPPost specifies the http Post request to perform.
	// +optional
	HTTPPost *HTTPPostAction `json:"httpPost,omitempty"`
	// TCPSocket specifies an action involving a TCP port.
	// TCP hooks not yet supported
	// TODO: implement a realistic TCP lifecycle hook
	// +optional
	TCPSocket *core.TCPSocketAction `json:"tcpSocket,omitempty"`
}

// HTTPPostAction describes an action based on HTTP Post requests.
type HTTPPostAction struct {
	// Path to access on the HTTP server.
	// +optional
	Path string `json:"path,omitempty"`
	// Name or number of the port to access on the container.
	// Number must be in the range 1 to 65535.
	// Name must be an IANA_SVC_NAME.
	Port intstr.IntOrString `json:"port"`
	// Host name to connect to, defaults to the pod IP. You probably want to set
	// "Host" in httpHeaders instead.
	// +optional
	Host string `json:"host,omitempty"`
	// Scheme to use for connecting to the host.
	// Defaults to HTTP.
	// +optional
	Scheme core.URIScheme `json:"scheme,omitempty"`
	// Custom headers to set in the request. HTTP allows repeated headers.
	// +optional
	HTTPHeaders []core.HTTPHeader `json:"httpHeaders,omitempty"`
	// Body to set in the request.
	// +optional
	Body string `json:"body,omitempty"`
}
