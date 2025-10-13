#!/bin/bash
./build.sh
oc create -f manifest.yaml
