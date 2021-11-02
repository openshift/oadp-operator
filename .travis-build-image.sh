#!/bin/bash

if [ "${TRAVIS_BRANCH}" == "${DEFAULT_BRANCH}" ]; then
  export TAG=latest
else
  export TAG=${TRAVIS_BRANCH}
fi

export ARCH=$(uname -m)

docker build -t ${IMAGE}:${TAG}-${ARCH} -f ${DOCKERFILE} .
docker login quay.io -u "${QUAY_ROBOT}" -p ${QUAY_TOKEN}
docker push ${IMAGE}:${TAG}-${ARCH}
