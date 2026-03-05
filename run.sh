#!/usr/bin/env bash
sudo docker run --memory=$1m --memory-swap=$1m a
RET=$?
echo "=================================================="
echo "ret: $RET"
