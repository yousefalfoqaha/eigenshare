#!/bin/bash

INPUT="$1"
CHUNK_SIZE=$((5 * 1024 * 1024))
HANDLE="test-$RANDOM"

if [ -d "$INPUT" ]; then
  FILE="/tmp/$(basename $INPUT).zip"
  zip -r "$FILE" "$INPUT"
else
  FILE="$INPUT"
fi

FILE_SIZE=$(stat -c%s "$FILE")
FILE_NAME=$(basename $FILE)

RESPONSE=$(curl -s -X POST http://localhost:8080/shares \
  -H "Content-Type: application/json" \
  -d '{
    "handle": "'$HANDLE'",
    "files": [{
      "name": "'$FILE_NAME'",
      "size": '$FILE_SIZE',
      "index": 0
    }]
  }')

SHARE_ID=$(echo $RESPONSE | jq -r '.id')
if [ -z "$SHARE_ID" ] || [ "$SHARE_ID" = "null" ]; then
  echo "Failed to create share"
  echo $RESPONSE | jq
  exit 1
fi

echo $RESPONSE | jq

CHUNK_INDEX=0
OFFSET=0

while [ $OFFSET -lt $FILE_SIZE ]; do
  echo "Uploading chunk $CHUNK_INDEX (offset $OFFSET)..."

  dd if="$FILE" bs=$CHUNK_SIZE skip=$CHUNK_INDEX count=1 2>/dev/null | \
    curl -s -X POST "http://localhost:8080/shares/upload?shareId=$SHARE_ID" \
      -H "X-Chunk-Index: $CHUNK_INDEX" \
      -H "X-File-Index: 0" \
      -H "Content-Type: application/octet-stream" \
      --data-binary @-

  OFFSET=$((OFFSET + CHUNK_SIZE))
  CHUNK_INDEX=$((CHUNK_INDEX + 1))
done

[ -d "$INPUT" ] && rm "$FILE"

echo "Done"
