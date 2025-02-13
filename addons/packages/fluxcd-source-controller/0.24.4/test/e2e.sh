#!/bin/bash
#!/bin/sh
#!/usr/bin/env bash

# Copyright 2021 VMware Tanzu Community Edition contributors. All Rights Reserved.
# SPDX-License-Identifier: Apache-2.0

set -e

# Checking package is installed or not
tanzu package installed list | grep "fluxcd-source-controller.community.tanzu.vmware.com" || {
  version=$(tanzu package available list fluxcd-source-controller.community.tanzu.vmware.com | tail -n 1 | awk '{print $2}')
  tanzu package install fluxcd-source-controller --package-name fluxcd-source-controller.community.tanzu.vmware.com --version "${version}"
}


function cleanup(){
    EXIT_CODE="$?"

    # only dump all logs if an error has occurred
    if [ ${EXIT_CODE} -ne 0 ]; then
        kubectl -n kube-system describe pods
        kubectl -n source-system describe pods
        kubectl -n source-system get gitrepositories -oyaml
        kubectl -n source-system get helmrepositories -oyaml
        kubectl -n source-system get helmcharts -oyaml
        kubectl -n source-system get all
        kubectl -n source-system logs deploy/source-controller
        kubectl -n minio get all
        kubectl -n minio describe pods
    else
        echo "All E2E tests passed!"
    fi
    exit ${EXIT_CODE}
}
trap cleanup EXIT

echo "Run smoke tests"
kubectl -n source-system apply -f "flux-source-controller/config/samples"
kubectl -n source-system rollout status deploy/source-controller --timeout=1m
kubectl -n source-system wait gitrepository/gitrepository-sample --for=condition=ready --timeout=1m
kubectl -n source-system wait helmrepository/helmrepository-sample --for=condition=ready --timeout=1m
kubectl -n source-system wait helmchart/helmchart-sample --for=condition=ready --timeout=1m
kubectl -n source-system delete -f "flux-source-controller/config/samples"

echo "Run HelmChart values file tests"
kubectl -n source-system apply -f "flux-source-controller/config/testdata/helmchart-valuesfile"
kubectl -n source-system wait helmchart/podinfo --for=condition=ready --timeout=5m
kubectl -n source-system wait helmchart/podinfo-git --for=condition=ready --timeout=5m
kubectl -n source-system delete -f "flux-source-controller/config/testdata/helmchart-valuesfile"

# Deleting package
tanzu package installed list | grep "fluxcd-source-controller.community.tanzu.vmware.com" || {
  version=$(tanzu package available list fluxcd-source-controller.community.tanzu.vmware.com | tail -n 1 | awk '{print $2}')
  tanzu package installed delete fluxcd-source-controller -y
}
