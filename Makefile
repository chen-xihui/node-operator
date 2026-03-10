# Makefile for node-operator

# Image URL to use all building/pushing image targets
IMG ?= controller:latest

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

test:
	go test ./... -coverprofile cover.out

##@ Development

manifests: controller-gen
	$(CONTROLLER_GEN) crd:trivialVersions=true paths="./api/..." output:crd:artifacts:config=config/crd/bases

generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./api/..."

fmt: goimports
	$(GOIMPORTS) -w ./api ./controllers ./cmd

##@ Build

build: generate fmt
	go build -o bin/manager ./cmd/manager

run: generate fmt manifests
	go run ./cmd/manager

##@ Deployment

install: manifests kustomize
	$(KUSTOMIZE) build config/default | kubectl apply -f -

uninstall: manifests kustomize
	$(KUSTOMIZE) build config/default | kubectl delete -f -

deploy: manifests kustomize
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | kubectl apply -f -

##@ Build Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
GOIMPORTS ?= $(LOCALBIN)/goimports

## Tool Versions
KUSTOMIZE_VERSION ?= v4.5.7
CONTROLLER_TOOLS_VERSION ?= v0.9.2

kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	curl -sSLo $(LOCALBIN)/kustomize https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2F$(KUSTOMIZE_VERSION)/kustomize_$(KUSTOMIZE_VERSION)_windows_amd64.exe
	chmod +x $(LOCALBIN)/kustomize

controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	curl -sSLo $(LOCALBIN)/controller-gen https://github.com/kubernetes-sigs/controller-tools/releases/download/$(CONTROLLER_TOOLS_VERSION)/controller-tools_$(CONTROLLER_TOOLS_VERSION)_windows_amd64.exe
	chmod +x $(LOCALBIN)/controller-gen

goimports: $(GOIMPORTS) ## Download goimports locally if necessary.
$(GOIMPORTS): $(LOCALBIN)
	go install golang.org/x/tools/cmd/goimports@latest
