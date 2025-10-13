#!/bin/bash

rm manifest.yaml || true
for f in manifests/*.yaml; do (cat "${f}"; echo) >> manifest.yaml; done
sed -i '$ d' manifest.yaml
