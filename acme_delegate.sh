#!/bin/bash
set -euo pipefail

DNSAPI="$1"
ACTION="$2"
DOMAIN="$3"
TXTVALUE="$4"

if [[ ! "$DNSAPI" =~ ^[a-zA-Z0-9_]+$ ]]; then
    echo "ERROR: invalid dnsapi name: $DNSAPI" >&2
    echo "ACME_RETVAL1ACME_RETVAL"
    exit 1
fi

export HOME=/opt
export DEBUG="${DEBUG:-3}"
cd /opt

echo "=== acme_delegate: start dnsapi=$DNSAPI action=$ACTION domain=$DOMAIN ==="
export

echo "=== acme_delegate: loading acme.sh ==="
source /opt/.acme.sh/acme.sh --info

echo "=== acme_delegate: loading dnsapi module $DNSAPI ==="
source /opt/.acme.sh/dnsapi/${DNSAPI}.sh

echo "=== acme_delegate: executing ${DNSAPI}_${ACTION} ==="
"${DNSAPI}_${ACTION}" "$DOMAIN" "$TXTVALUE"
retval=$?

echo "=== acme_delegate: finished retval=$retval ==="
echo "ACME_RETVAL${retval}ACME_RETVAL"