#!/usr/bin/env bash
if [ "$TIME_ZONE" != "" ]; then
    ln -snf /usr/share/zoneinfo/$TIME_ZONE /etc/localtime && echo $TIME_ZONE > /etc/timezone
fi

/opt/apps/zta -c /opt/apps/zta/etc/gateway.yaml