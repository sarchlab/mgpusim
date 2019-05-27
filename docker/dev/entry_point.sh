#!/bin/bash

# when no arguments given exit with errcode 1
if [ -z "$@" ]
then
   >&2 echo "ERROR: You need to provide Gitlab logins!"
   exit 1
fi

mkdir /root/.ssh && touch /root/.ssh/authroized_keys
for user in $@
do
  echo "Fetching key for $user..."
  # fetch pubkeys by Gitlab user login
  curl https://gitlab.com/$user.keys >> $user.pubkey
  cmd="cd /root/dev/src/gitlab.com/akita"
  sed -i "s|^|command=\"$cmd\" |g" $user.pubkey
  cat $user.pubkey >> /root/.ssh/authorized_keys
done

mkdir /run/sshd
# start ssh daemon
exec /usr/sbin/sshd -Def /etc/ssh/sshd_config