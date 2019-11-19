#!/bin/bash

set -e

which jq &>/dev/null || { echo "Please install jq (https://stedolan.github.io/jq/)."; exit 1; }

CERT_FILE_PATH=$1
if [ "${CERT_FILE_PATH}" == "" ]; then
  echo "Must specify a file path to the file that has the keys/certs"
  exit 1
fi

MANIFEST_SECRET_YAML=$2
if [ "${MANIFEST_SECRET_YAML}" == "" ]; then
  echo "Must specify a path to the yaml file for Secret object"
  exit 1
fi

MANIFEST_APISERVICE_YAML=$3
if [ "${MANIFEST_APISERVICE_YAML}" == "" ]; then
  echo "Must specify a path to the yaml file for APIService object"
  exit 1
fi

MANIFEST_MUTATING_WEBHOOK_YAML=$4
if [ "${MANIFEST_MUTATING_WEBHOOK_YAML}" == "" ]; then
  echo "Must specify a path to the yaml file for MutatingWebhookConfiguration object"
  exit 1
fi

KUBE_CA=$(cat ${CERT_FILE_PATH} | jq '."kube.ca"')
TLS_SERVING_CERT=$(cat ${CERT_FILE_PATH} | jq '."tls.serving.cert"')
TLS_SERVING_KEY=$(cat ${CERT_FILE_PATH} | jq '."tls.serving.key"')
SERVICE_SERVING_CERT_CA=$(cat ${CERT_FILE_PATH} | jq '."service.serving.cert.ca"')

sed "s/TLS_SERVING_CERT/${TLS_SERVING_CERT}/g" -i "${MANIFEST_SECRET_YAML}"
sed "s/TLS_SERVING_KEY/${TLS_SERVING_KEY}/g" -i "${MANIFEST_SECRET_YAML}"

sed "s/SERVICE_SERVING_CERT_CA/${SERVICE_SERVING_CERT_CA}/g" -i "${MANIFEST_APISERVICE_YAML}"
sed "s/KUBE_CA/${KUBE_CA}/g" -i "${MANIFEST_MUTATING_WEBHOOK_YAML}"
