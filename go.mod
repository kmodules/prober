module kmodules.xyz/prober

go 1.12

require (
	github.com/appscode/go v0.0.0-20200323182826-54e98e09185a
	github.com/gabriel-vasile/mimetype v1.1.0
	github.com/go-openapi/spec v0.19.3
	github.com/gogo/protobuf v1.3.1
	github.com/stretchr/testify v1.4.0
	k8s.io/api v0.18.3
	k8s.io/apimachinery v0.18.3
	k8s.io/client-go v0.18.3
	k8s.io/kube-openapi v0.0.0-20200410145947-61e04a5be9a6
	k8s.io/utils v0.0.0-20200324210504-a9aa75ae1b89
	kmodules.xyz/client-go v0.0.0-20200521065424-173e32c78a20
)

replace (
	k8s.io/apimachinery => github.com/kmodules/apimachinery v0.19.0-alpha.0.0.20200520235721-10b58e57a423
	k8s.io/apiserver => github.com/kmodules/apiserver v0.18.4-0.20200521000930-14c5f6df9625
	k8s.io/kubernetes => github.com/kmodules/kubernetes v1.19.0-alpha.0.0.20200521033432-49d3646051ad
)
