#!/bin/bash

oc delete -f manifest.yaml
oc delete project parks-app
