#!/bin/bash

# when no arguments given exit with errcode 1
if [ -z "$@" ]
then
   >&2 echo "ERROR: You need to provide Gitlab logins!"
   exit 1
fi

docker run -d -P registry.gitlab.com/akita/gcn3 $@
name=$(docker ps -l --format "{{.ID}}")
ip=$(docker port $name 22/tcp)
port=$(echo $ip | awk '{split($0, a, ":"); print a[2]}')
sleep 2
ssh -oStrictHostKeyChecking=accept-new -p $port root@0.0.0.0

