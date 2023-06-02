IMAGE_REGISTRY ?= quay.io
IMAGE_TAG ?= latest
IMAGE_NAME ?= oadp/must-gather

PROMETHEUS_LOCAL_DATA_DIR ?= /tmp/oadp-data-dump
# Search for prom_data.tar.gz archive in must-gather output in currect directory by default
PROMETHEUS_DUMP_PATH ?= $(shell find ./must-gather.local* -name prom_data.tar.gz -printf "%T@ %p\n" | sort -n | tail -1 | cut -d" " -f2)

build: docker-build docker-push

run: IMAGE_REGISTRY:=ttl.sh
run: IMAGE_NAME:=oadp/must-gather-$(shell git rev-parse --short HEAD)-$(shell echo $$RANDOM)
run: IMAGE_TAG:=1h
run:
	IMAGE_REGISTRY=$(IMAGE_REGISTRY) IMAGE_NAME=$(IMAGE_NAME) IMAGE_TAG=$(IMAGE_TAG) make build && \
	oc adm must-gather --image ${IMAGE_REGISTRY}/${IMAGE_NAME}:${IMAGE_TAG}

PLATFORM ?= linux/amd64
docker-build:
	docker build --platform=${PLATFORM} -t ${IMAGE_REGISTRY}/${IMAGE_NAME}:${IMAGE_TAG} .

docker-push:
	docker push ${IMAGE_REGISTRY}/${IMAGE_NAME}:${IMAGE_TAG}

.PHONY: build docker-build docker-push

prometheus-run: prometheus-cleanup-container prometheus-load-dump
	docker run -d \
	  --mount type=bind,source=${PROMETHEUS_LOCAL_DATA_DIR},target=/prometheus \
	  --name oadp-prometheus \
	  --publish 127.0.0.1:9090:9090 \
	  prom/prometheus:v2.21.0 \
	&& echo "Started Prometheus on http://localhost:9090"

prometheus-load-dump: prometheus-check-archive-file prometheus-cleanup-data
	mkdir -p ${PROMETHEUS_LOCAL_DATA_DIR}
	tar xvf ${PROMETHEUS_DUMP_PATH} -C ${PROMETHEUS_LOCAL_DATA_DIR} --strip-components=1 --no-same-owner
	chmod -R 777 ${PROMETHEUS_LOCAL_DATA_DIR}

prometheus-cleanup-container:
	# delete data files directly from the container to allow delete data directory from outside of the container
	docker exec oadp-prometheus rm -rf /prometheus || true
	docker rm -f oadp-prometheus || true

prometheus-cleanup-data:
	rm -rf ${PROMETHEUS_LOCAL_DATA_DIR}

prometheus-cleanup: prometheus-cleanup-container prometheus-cleanup-data

prometheus-check-archive-file:
	test -f "${PROMETHEUS_DUMP_PATH}" || (echo "Error: Prometheus archive file does not exist. Specify valid file in PROMETHEUS_DUMP_PATH environment variable."; exit 1)
