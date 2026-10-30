#!/bin/sh
# Issues the lite stack's internal mTLS material: a private CA (key kept in CA_DIR, never mounted into services),
# the api server certificate and one client certificate per workload. Valid files are never regenerated; a leaf
# is re-issued only when missing, close to expiry or not signed by the current CA. Idempotent, runs on every up.
set -eu

CERTS_DIR="${CERTS_DIR:-/certs}"
CA_DIR="${CA_DIR:-/ca}"
CERT_OWNER="${CERT_OWNER:-65532:65532}"
CA_DAYS="${CA_DAYS:-3650}"
LEAF_DAYS="${LEAF_DAYS:-825}"
RENEW_BEFORE_SECONDS="${RENEW_BEFORE_SECONDS:-2592000}"
KEY_ALGORITHM="rsa:2048"
DIGEST="-sha256"
SERIAL_BYTES=16
PUBLIC_MODE=0644
KEY_MODE=0400
DIR_MODE=0755
ROOT_UID=0
CA_SUBJECT="/O=event-router-lite/CN=event-router-lite internal CA"
SERVER_NAME="api"
SERVER_SANS="DNS:api,DNS:localhost,IP:127.0.0.1"
CLIENT_NAMES="worker preview"
CA_KEY="$CA_DIR/ca.key"
CA_CERT="$CA_DIR/ca.crt"
PUBLIC_CA_CERT="$CERTS_DIR/ca.crt"

log() {
  echo "certs-init: $*"
}

work_dir=$(mktemp -d)
trap 'rm -rf "$work_dir"' EXIT INT TERM
umask 077
mkdir -p "$CERTS_DIR" "$CA_DIR"

ca_is_usable() {
  [ -s "$CA_KEY" ] && [ -s "$CA_CERT" ] && openssl x509 -checkend "$RENEW_BEFORE_SECONDS" -noout -in "$CA_CERT" >/dev/null
}

create_ca() {
  cfg="$work_dir/ca.cnf"
  cat >"$cfg" <<EOF
[req]
distinguished_name = dn
prompt = no
[dn]
[v3_ca]
basicConstraints = critical,CA:TRUE,pathlen:0
keyUsage = critical,keyCertSign,cRLSign
subjectKeyIdentifier = hash
EOF
  openssl req -x509 -new -newkey "$KEY_ALGORITHM" -nodes -config "$cfg" -extensions v3_ca \
    -subj "$CA_SUBJECT" -days "$CA_DAYS" $DIGEST -keyout "$work_dir/ca.key" -out "$work_dir/ca.crt" 2>/dev/null
  mv "$work_dir/ca.key" "$CA_KEY"
  mv "$work_dir/ca.crt" "$CA_CERT"
  log "created internal CA (valid $CA_DAYS days)"
}

publish_ca() {
  if ! cmp -s "$CA_CERT" "$PUBLIC_CA_CERT" 2>/dev/null; then
    cp "$CA_CERT" "$work_dir/ca.pub"
    chmod "$PUBLIC_MODE" "$work_dir/ca.pub"
    mv "$work_dir/ca.pub" "$PUBLIC_CA_CERT"
    log "published CA certificate"
  fi
}

leaf_is_valid() {
  crt="$CERTS_DIR/$1.crt"
  [ -s "$crt" ] && [ -s "$CERTS_DIR/$1.key" ] &&
    openssl x509 -checkend "$RENEW_BEFORE_SECONDS" -noout -in "$crt" >/dev/null &&
    openssl verify -CAfile "$PUBLIC_CA_CERT" "$crt" >/dev/null 2>&1
}

write_leaf_extensions() {
  usage="$2"
  sans="$3"
  {
    echo "basicConstraints = critical,CA:FALSE"
    echo "keyUsage = critical,digitalSignature,keyEncipherment"
    echo "extendedKeyUsage = $usage"
    echo "authorityKeyIdentifier = keyid"
    if [ -n "$sans" ]; then
      echo "subjectAltName = $sans"
    fi
  } >"$work_dir/$1.ext"
}

install_leaf() {
  if [ "$(id -u)" = "$ROOT_UID" ]; then
    chown "$CERT_OWNER" "$work_dir/$1.key"
  fi
  chmod "$KEY_MODE" "$work_dir/$1.key"
  chmod "$PUBLIC_MODE" "$work_dir/$1.crt"
  mv -f "$work_dir/$1.key" "$CERTS_DIR/$1.key"
  mv -f "$work_dir/$1.crt" "$CERTS_DIR/$1.crt"
}

issue_leaf() {
  name="$1"
  if leaf_is_valid "$name"; then
    log "$name certificate is valid, kept"
    return 0
  fi
  write_leaf_extensions "$name" "$2" "$3"
  openssl req -new -newkey "$KEY_ALGORITHM" -nodes -subj "/O=event-router-lite/CN=$name" \
    -keyout "$work_dir/$name.key" -out "$work_dir/$name.csr" 2>/dev/null
  openssl x509 -req -in "$work_dir/$name.csr" -CA "$CA_CERT" -CAkey "$CA_KEY" \
    -set_serial "0x$(openssl rand -hex "$SERIAL_BYTES")" -days "$LEAF_DAYS" $DIGEST \
    -extfile "$work_dir/$name.ext" -out "$work_dir/$name.crt" 2>/dev/null
  install_leaf "$name"
  log "issued $name certificate (valid $LEAF_DAYS days)"
}

if ! ca_is_usable; then
  create_ca
fi
publish_ca
chmod "$DIR_MODE" "$CERTS_DIR"
issue_leaf "$SERVER_NAME" serverAuth "$SERVER_SANS"
for client in $CLIENT_NAMES; do
  issue_leaf "$client" clientAuth ""
done
log "done"
