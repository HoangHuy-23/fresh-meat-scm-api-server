#!/usr/bin/env bash
# Usage : ./scripts/check-attrs.sh ./wallet/User1@org1.id

set -e

if [ $# -ne 1 ]; then
  echo "❌ Usage: $0 <path-to-.id-file>"
  exit 1
fi

ID_FILE=$1

if [ ! -f "$ID_FILE" ]; then
  echo "❌ File not found: $ID_FILE"
  exit 1
fi

CERT_FILE=$(mktemp)

# Extract certificate properly: remove quotes, replace \n with real newlines
jq -r '.credentials.certificate' "$ID_FILE" \
  | sed 's/\\n/\n/g' \
  | sed 's/^"//;s/"$//' > "$CERT_FILE"

# Verify cert
if ! openssl x509 -in "$CERT_FILE" -noout >/dev/null 2>&1; then
  echo "❌ Failed to parse certificate from $ID_FILE"
  echo "👉 Debug: try 'jq -r .credentials.certificate $ID_FILE' to inspect"
  rm -f "$CERT_FILE"
  exit 1
fi

echo "🔎 Checking attrs in $ID_FILE ..."
echo "--------------------------------------------"

# Extract raw extension (OID with attrs JSON)
ATTRS_JSON=$(openssl x509 -in "$CERT_FILE" -noout -text \
  | awk '/1\.2\.3\.4\.5\.6\.7\.8\.1:/{getline; print $0}' \
  | sed 's/^[ \t]*//')

if [ -z "$ATTRS_JSON" ]; then
  echo "⚠️ No attributes found in certificate"
else
  echo "📜 Raw attrs JSON:"
  echo "$ATTRS_JSON"
  echo
  echo "✅ Parsed attrs:"
  echo "$ATTRS_JSON" | jq -r '.attrs | to_entries[] | "\(.key)=\(.value)"'
fi

echo "--------------------------------------------"

rm -f "$CERT_FILE"
