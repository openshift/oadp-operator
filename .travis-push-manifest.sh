#!/bin/bash
if [[ -n "${QUAY_ROBOT}" ]]; then

  if [ "${TRAVIS_BRANCH}" == "${DEFAULT_BRANCH}" ]; then
    export TAG=latest
  else
    export TAG=${TRAVIS_BRANCH}
  fi

  export DOCKER_CLI_EXPERIMENTAL=enabled

  #Without this docker manifest create fails
  #https://github.com/docker/for-linux/issues/396
  sudo chmod o+x /etc/docker

  docker manifest create \
    ${IMAGE}:${TAG} \
    ${IMAGE}:${TAG}-x86_64 \
    ${IMAGE}:${TAG}-ppc64le \
    ${IMAGE}:${TAG}-s390x \
    ${IMAGE}:${TAG}-aarch64

  docker manifest inspect ${IMAGE}:${TAG}

  docker login quay.io -u "${QUAY_ROBOT}" -p ${QUAY_TOKEN}

  docker manifest push ${IMAGE}:${TAG}
fi
