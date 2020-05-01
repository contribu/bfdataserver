#!/bin/bash

mkdir -p built
go build -o built/bfdataserver ./cmd/bfdataserver
