#!/usr/bin/env bash

set -e

ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

if [[ -z $SUBSTREAMS_API_TOKEN ]]; then
  echo "Please set SUBSTREAMS_API_TOKEN in your environment"
  exit 1
fi

echo ""
echo "----- Running docker environment -----"
echo ""
sleep 1
docker compose -f $ROOT/dev-environment/docker-compose.yml up -d --wait

echo ""
echo "----- Installing npm dependencies -----"
echo ""
sleep 1
npm install

echo ""
echo "----- Generating bindings -----"
echo ""
sleep 1
npm run generate

echo ""
echo "----- Generating codegen -----"
echo ""
sleep 1
npm run codegen

echo ""
echo "----- Creating local graph -----"
echo ""
npm run create-local

echo ""
echo "----- Running local graph -----"
echo ""
sleep 1
npm run deploy-local
