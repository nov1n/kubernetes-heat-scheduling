#!/bin/bash

BIN_PATH="../../../../../k8s.io/kubernetes/_output/local/bin/linux/amd64"
sudo -E "${BIN_PATH}/hyperkube" scheduler \
      --v=5 \
      --master="http://localhost:8080" \
      --scheduler-name="heat-scheduler" \
      --port=10253 \
      --policy-config-file="policy-config-file.json"
