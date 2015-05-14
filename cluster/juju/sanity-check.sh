#!/bin/bash

set -e

export KUBERNETES_PROVIDER=juju
make clean
make all WHAT=cmd/kubectl
cluster/juju/kube-up.sh
juju do action petstore
