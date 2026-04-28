#!/bin/bash

set -euo pipefail

CHUNK_SIZE=$((5 * 1024 * 1024))
HANDLE="test-$RANDOM"
MAX_PARALLEL=4

if [ $# -eq 0 ]; then
  echo "Usage: $0 <file_or_dir> [file_or_dir ...]"
  exit 1
fi

declare -a FILES FILE_NAMES FILE_SIZES IS_TEMP
FILE_INDEX=0

for INPUT in "$@"; do
  if [ ! -e "$INPUT" ]; then
    echo "Skipping non-existent: $INPUT"
    continue
  fi

  if [ -d "$INPUT" ]; then
    ZIP="/tmp/$(basename "$INPUT").zip"
    zip -r "$ZIP" "$INPUT"
    FILES[$FILE_INDEX]="$ZIP"
    FILE_NAMES[$FILE_INDEX]="$(basename "$ZIP")"
    IS_TEMP[$FILE_INDEX]=1
  else
    FILES[$FILE_INDEX]="$INPUT"
    FILE_NAMES[$FILE_INDEX]="$(basename "$INPUT")"
    IS_TEMP[$FILE_INDEX]=0
  fi

  FILE_SIZES[$FILE_INDEX]=$(stat -c%s "${FILES[$FILE_INDEX]}")
  FILE_INDEX=$((FILE_INDEX + 1))
done

if [ $FILE_INDEX -eq 0 ]; then
  echo "No valid inputs provided"
  exit 1
fi

TOTAL_FILES=$FILE_INDEX
TOTAL_SIZE=0
for i in $(seq 0 $((TOTAL_FILES - 1))); do
  TOTAL_SIZE=$((TOTAL_SIZE + FILE_SIZES[$i]))
done

echo "Creating share with $TOTAL_FILES file(s) ($(( TOTAL_SIZE / 1024 / 1024 ))MB total)"

FILES_JSON=""
for i in $(seq 0 $((TOTAL_FILES - 1))); do
  [ $i -gt 0 ] && FILES_JSON+=","
  FILES_JSON+='{"name":"'${FILE_NAMES[$i]}'","size":'${FILE_SIZES[$i]}',"index":'$i'}'
done

RESPONSE=$(curl -s -X POST http://localhost:8080/shares \
  -H "Content-Type: application/json" \
  -d '{"handle":"'$HANDLE'","files":['$FILES_JSON']}')

SHARE_ID=$(echo "$RESPONSE" | jq -r '.id')
if [ -z "$SHARE_ID" ] || [ "$SHARE_ID" = "null" ]; then
  echo "Failed to create share"
  echo "$RESPONSE" | jq
  exit 1
fi

echo "Share created: $SHARE_ID"
echo "$RESPONSE" | jq

TOTAL_CHUNKS=0
declare -a FILE_CHUNKS
for i in $(seq 0 $((TOTAL_FILES - 1))); do
  FILE_CHUNKS[$i]=$(( (FILE_SIZES[$i] + CHUNK_SIZE - 1) / CHUNK_SIZE ))
  TOTAL_CHUNKS=$(( TOTAL_CHUNKS + FILE_CHUNKS[$i] ))
done

upload_chunk() {
  local file_idx=$1
  local chunk_idx=$2
  local file_path="${FILES[$file_idx]}"

  dd if="$file_path" bs=$CHUNK_SIZE skip=$chunk_idx count=1 2>/dev/null | \
    curl -s -X POST "http://localhost:8080/shares/upload?shareId=$SHARE_ID" \
      -H "X-Chunk-Index: $chunk_idx" \
      -H "X-File-Index: $file_idx" \
      -H "Content-Type: application/octet-stream" \
      --data-binary @-
}

echo ""
echo "Uploading $TOTAL_CHUNKS chunks across $TOTAL_FILES files (max $MAX_PARALLEL parallel)..."

declare -a JOB_FILE JOB_CHUNK
UPLOADED=0
QUEUE=()

enqueue() {
  local file_idx=$1
  local chunk_idx=$2
  upload_chunk "$file_idx" "$chunk_idx" &
  QUEUE+=("$!")
  JOB_FILE["$!"]="$file_idx"
  JOB_CHUNK["$!"]="$chunk_idx"
}

drain_one() {
  local pid="${QUEUE[0]}"
  QUEUE=("${QUEUE[@]:1}")
  wait "$pid"
  UPLOADED=$((UPLOADED + 1))
  local pct=$(( UPLOADED * 100 / TOTAL_CHUNKS ))
  echo "[$(printf '%3d' $pct)%] $UPLOADED/$TOTAL_CHUNKS chunks — file ${FILE_NAMES[${JOB_FILE[$pid]}]} chunk ${JOB_CHUNK[$pid]}"
}

drain_all() {
  for pid in "${QUEUE[@]}"; do
    wait "$pid"
    UPLOADED=$((UPLOADED + 1))
    local pct=$(( UPLOADED * 100 / TOTAL_CHUNKS ))
    echo "[$(printf '%3d' $pct)%] $UPLOADED/$TOTAL_CHUNKS chunks — file ${FILE_NAMES[${JOB_FILE[$pid]}]} chunk ${JOB_CHUNK[$pid]}"
  done
  QUEUE=()
}

CHUNK_LIST=$(mktemp)
for i in $(seq 0 $((TOTAL_FILES - 1))); do
  for c in $(seq 0 $((FILE_CHUNKS[$i] - 1))); do
    echo "$i $c" >> "$CHUNK_LIST"
  done
done
SHUFFLED=$(shuf "$CHUNK_LIST")
rm -f "$CHUNK_LIST"

while read -r file_idx chunk_idx; do
  enqueue "$file_idx" "$chunk_idx"

  if [ ${#QUEUE[@]} -ge $MAX_PARALLEL ]; then
    drain_one
  fi
done <<< "$SHUFFLED"

drain_all

echo ""
echo ""

for i in $(seq 0 $((TOTAL_FILES - 1))); do
  [ "${IS_TEMP[$i]}" -eq 1 ] && rm -f "${FILES[$i]}"
done

echo "Done — share $SHARE_ID is complete"
