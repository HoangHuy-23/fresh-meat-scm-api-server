#!/bin/bash
# Usage: ./check-attrs.sh <identity.id>

if [ $# -lt 1 ]; then
  echo "Usage: $0 <identity.id>"
  exit 1
fi

ID_FILE=$1
CERT_FILE=$(mktemp)

# Extract certificate từ .id
jq -r '.credentials.certificate' "$ID_FILE" | sed 's/\\n/\n/g' > "$CERT_FILE"

# Parse attrs JSON từ cert và in gọn
ATTRS_JSON=$(openssl x509 -in "$CERT_FILE" -noout -text \
  | awk '/1\.2\.3\.4\.5\.6\.7\.8\.1/{flag=1;next}/Signature Algorithm/{flag=0}flag' \
  | tr -d ' ')

if [ -n "$ATTRS_JSON" ]; then
  echo "✅ Attributes:"
  echo "$ATTRS_JSON" | jq -r '.attrs | to_entries[] | "- \(.key): \(.value)"'
else
  echo "⚠️ No attributes found"
fi

