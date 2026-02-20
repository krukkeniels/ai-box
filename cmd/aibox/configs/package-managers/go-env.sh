#!/bin/sh
# AI-Box Go module proxy configuration
# Sourced from /etc/profile.d/ to configure Go module downloads via Nexus

export GOPROXY="https://nexus.internal/repository/go-proxy/,direct"
export GONOSUMDB="*"
export GOFLAGS=""
