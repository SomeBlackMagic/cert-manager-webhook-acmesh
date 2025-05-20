#!/bin/bash

echo '__________________________ acme_delegate initialize __________________________'
export HOME=/opt
export DEBUG=3
cd /opt
export

echo '__________________________ acme.sh environments __________________________'
dosk_dnsapi="$1"
dosk_action="$2"
dosk_fulldomain="$3"
dosk_txtvalue="$4"
dosk_lasterror=""

echo '__________________________ acme.sh delegate __________________________'
source /opt/.acme.sh/acme.sh --info
source /opt/.acme.sh/dnsapi/${dosk_dnsapi}.sh
${dosk_dnsapi}_${dosk_action} "$dosk_fulldomain" "$dosk_txtvalue"
retval=$?

echo '__________________________ acme.sh finish __________________________'
echo "ACME_RETVAL${retval}ACME_RETVAL"