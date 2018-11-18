#!/bin/sh
set -e
cd "$(dirname "$0")"

hostname=localhost
certfile=$hostname.cert
keyfile=$hostname.key

if [ -f "$certfile" ] && [ -f "$keyfile" ]; then
  echo "$certfile and $keyfile already exists."
else
  rm -f .csr.pem
  echo "Generating cert & key files ($certfile & $keyfile)"
  openssl req \
    -nodes -newkey rsa:2048 \
    -keyout "$keyfile" \
    -sha256 \
    -out .csr.pem \
    -subj "/C=US/ST=California/L=San Francisco/O=/OU=/CN=$hostname"
  openssl x509 -req -in .csr.pem -signkey "$keyfile" -out "$certfile"
  rm .csr.pem

  echo "You should manually add the local hostname to /etc/hosts:"
  echo "sudo echo '127.0.0.1 $hostname' >> /etc/hosts"
fi

if (uname | grep -i darwin >/dev/null); then
  /bin/echo -n "Add the cert to your macOS keychain? [Y/n] "
  read answer
  if [[ "$answer" == "" ]] || [[ "$answer" == "y" ]] || [[ "$answer" == "Y" ]]
  then
    set +e # allow user to cancel this
    security add-trusted-cert -d -r trustRoot \
      -k ~/Library/Keychains/login.keychain "$certfile"
    set -e
  fi
fi
