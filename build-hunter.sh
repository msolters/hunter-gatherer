#!/bin/bash
# GOOS and GOARCH must match the OS and architecture
# of the remote target; that is, where hunter is going
# to be executed!
# For most GCP instances, the defaults are linux and amd64.
# Other options are
#   GOOS=windows,darwin
#   GOARCH=386 // for x86
GOOS=linux GOARCH=amd64 go build hunter.go
