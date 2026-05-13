#!/usr/bin/env sh
set -e

DNSAPI="$1"
ACTION="$2"
DOMAIN="$3"
TXTVALUE="$4"

if ! echo "$DNSAPI" | grep -qE '^[a-zA-Z0-9_]+$'; then
    echo "ERROR: invalid dnsapi name: $DNSAPI" >&2
    echo "ACME_RETVAL1ACME_RETVAL"
    exit 1
fi

export DEBUG="${DEBUG:-3}"


echo "=== acme_delegate: start dnsapi=$DNSAPI action=$ACTION domain=$DOMAIN ==="

echo "=== acme_delegate: loading acme.sh ==="
. /acmebin/acme.sh --info

echo "=== acme_delegate: loading dnsapi module $DNSAPI ==="
. /acmebin/dnsapi/${DNSAPI}.sh

echo "=== acme_delegate: executing ${DNSAPI}_${ACTION} ==="
"${DNSAPI}_${ACTION}" "$DOMAIN" "$TXTVALUE"
retval=$?

echo "=== acme_delegate: finished retval=$retval ==="
echo "ACME_RETVAL${retval}ACME_RETVAL"
