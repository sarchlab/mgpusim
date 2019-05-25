#!/bin/bash

# when no arguments given exit with errcode 1
if [ -z "$@" ]
then
   >&2 echo "ERROR: You need to provide Gitlab logins!"
   exit 1
fi

for user in $@
do
  echo "Fetching key for $user..."
  # fetch pubkeys by Gitlab user login
  curl https://gitlab.com/$user.keys >> $user.pubkey
  sed -i 's|^|command="tmux -v new -s $user -t pair" |g' $user.pubkey
  cat $user.pubkey >> /home/akita/.ssh/authorized_keys
done

mkdir /run/sshd
# start ssh daemon
exec /usr/sbin/sshd -Def /etc/ssh/sshd_config