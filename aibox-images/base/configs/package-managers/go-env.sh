#!/bin/sh
# AI-Box Go environment configuration (open-source defaults)
# Enterprise deployments override GOPROXY to point to an internal Nexus mirror.
export GOPATH="/home/dev/go"
export GOPROXY="https://proxy.golang.org,direct"
export GONOSUMCHECK=""
export PATH="${GOPATH}/bin:${PATH}"
