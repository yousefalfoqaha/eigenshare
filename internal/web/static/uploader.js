async function streamFile(handle, fileID, file, onProgress) {
  const chunkSize = 1024 * 1024;
  let offset = 0;

  while (offset < file.size) {
    const end = Math.min(offset + chunkSize, file.size);
    const response = await fetch(`/shares/${handle}/files/${fileID}/stream`, {
      method: "PUT",
      headers: {
        "Content-Type": "application/octet-stream",
        "X-Write-Offset": String(offset),
      },
      body: await file.slice(offset, end).arrayBuffer(),
    });

    if (!response.ok) {
      const error = await response.json().catch(() => ({}));
      if (
        response.status === 409 &&
        Number.isSafeInteger(error.bytesWritten) &&
        error.bytesWritten > offset &&
        error.bytesWritten <= file.size
      ) {
        offset = error.bytesWritten;
        if (onProgress) onProgress(error);
        continue;
      }
      throw new Error(error.message || `upload failed: ${response.status}`);
    }

    const progress = await response.json();
    offset = progress.bytesWritten;
    if (onProgress) onProgress(progress);
  }
}
