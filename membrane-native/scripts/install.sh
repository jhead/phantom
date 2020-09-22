#!/bin/bash
set -ex

function setEnvs {
    export GOPATH=$(pwd)/.go
    export GO111MODULE=on

    export PHANTOM_PKG="github.com/jhead/phantom"
    export PHANTOM_GOPATH="${GOPATH}/src/${PHANTOM_PKG}"
    export MEMBRANE_GOPATH="${PHANTOM_GOPATH}/membrane"
}

function linkMembrane {
    go get golang.org/x/mobile/cmd/gomobile

    rm -rfv ${PHANTOM_GOPATH}
    mkdir -p ${PHANTOM_GOPATH}

    if [[ ! -z ${MEMBRANE_LOCAL} ]]; then
        # Used for make
        ln -s $(pwd)/../membrane ${MEMBRANE_GOPATH}
        # pushd ${MEMBRANE_GOPATH}
        # go mod download
        # popd
    else 
        # Used for npm
        ln -s $(pwd)/../membrane ${MEMBRANE_GOPATH}
        # go get -d ${MEMBRANE_PKG}
    fi
}

function bindMembrane {
    libName="Membrane.framework"

    pushd ${MEMBRANE_GOPATH}
    if [[ ! -e ${libName} ]]; then
        gomobile bind -target=ios ./cmd/api
    fi
    popd

    rm -rfv ios/${libName}
    mv ${MEMBRANE_GOPATH}/${libName} ios/
}

function buildMembrane {
    setEnvs
    (linkMembrane)
    (bindMembrane)
}

(buildMembrane)
