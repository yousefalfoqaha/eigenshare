function formatBytes(bytes) {
  const unit = 1024;
  if (bytes < unit) {
    return `${bytes} B`;
  }
  let div = unit;
  let exp = 0;
  for (let n = bytes / unit; n >= unit; n /= unit) {
    div *= unit;
    exp++;
  }
  return `${(bytes / div).toFixed(1)} ${"KMGTPE"[exp]}B`;
}

async function registerFiles(shareID, files) {
  const response = await fetch(`/api/shares/${shareID}/files`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      files: files.map((f) => ({
        name: f.name,
        expectedSize: f.size,
      })),
    }),
  });

  if (!response.ok) {
    const error = await response.json().catch(() => ({}));
    throw new Error(error.message || `failed to register files: ${response.status}`);
  }

  return (await response.json()) || [];
}

async function streamFile(shareID, fileID, file, onProgress) {
  const chunkSize = 1024 * 1024;
  let offset = 0;

  while (offset < file.size) {
    const end = Math.min(offset + chunkSize, file.size);
    const response = await fetch(`/api/shares/${shareID}/files/${fileID}/stream`, {
      method: "PUT",
      headers: {
        "Content-Type": "application/octet-stream",
        "Upload-Offset": String(offset),
      },
      body: await file.slice(offset, end).arrayBuffer(),
    });

    if (!response.ok) {
      const uploadOffset = Number(response.headers.get("Upload-Offset"));
      if (
        response.status === 409 &&
        Number.isSafeInteger(uploadOffset) &&
        uploadOffset > offset &&
        uploadOffset <= file.size
      ) {
        offset = uploadOffset;
        if (onProgress) onProgress(offset);
        continue;
      }
      const error = await response.json().catch(() => ({}));
      throw new Error(error.message || `upload failed: ${response.status}`);
    }

    const nextOffset = Number(response.headers.get("Upload-Offset"));
    if (
      !Number.isSafeInteger(nextOffset) ||
      nextOffset <= offset ||
      nextOffset > file.size
    ) {
      throw new Error("invalid Upload-Offset response header");
    }
    offset = nextOffset;
    if (onProgress) onProgress(offset);
  }
}
