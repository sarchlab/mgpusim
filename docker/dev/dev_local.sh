#!/bin/bash

# when no arguments given exit with errcode 1
if [ -z "$@" ]
then
   >&2 echo "ERROR: You need to provide Gitlab logins!"
   exit 1
fi

docker pull registry.gitlab.com/akita/mgpusim/dev:latest
docker run -d -P --security-opt=seccomp:unconfined registry.gitlab.com/akita/mgpusim/dev:latest $@
name=$(docker ps -l --format "{{.ID}}")
ip=$(docker port $name 22/tcp)
port=$(echo $ip | awk '{split($0, a, ":"); print a[2]}')
sleep 2
echo "Use the following command to connect to the dev server: "
echo "  ssh -p $port root@0.0.0.0"

