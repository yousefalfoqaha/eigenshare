function formatBytes(bytes) {
  const unit = 1024;
  if (bytes < unit) return `${bytes} B`;
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
      files: files.map((f) => ({ name: f.name, expectedSize: f.size })),
    }),
  });
  if (!response.ok) throw new Error(`failed to register: ${response.status}`);
  return (await response.json()) || [];
}

async function streamFile(shareID, fileID, file, onProgress, signal) {
  const chunkSize = 1024 * 1024;
  let offset = 0;
  while (offset < file.size) {
    const end = Math.min(offset + chunkSize, file.size);
    const response = await fetch(
      `/api/shares/${shareID}/files/${fileID}/stream`,
      {
        method: "PUT",
        headers: {
          "Content-Type": "application/octet-stream",
          "Upload-Offset": String(offset),
        },
        body: await file.slice(offset, end).arrayBuffer(),
        signal,
      },
    );
    if (!response.ok) {
      const uploadOffset = Number(response.headers.get("Upload-Offset"));
      if (response.status === 409 && uploadOffset > offset) {
        offset = uploadOffset;
        if (onProgress) onProgress(offset);
        continue;
      }
      throw new Error(`upload failed: ${response.status}`);
    }
    offset = Number(response.headers.get("Upload-Offset"));
    if (onProgress) onProgress(offset);
  }
}

class HeadlessQueue {
  constructor(callbacks) {
    this.shareID = shareID;
    this.tasks = new Map();
    this.isDraining = false;
    this.callbacks = callbacks;
  }

  seed(entries, progressMap) {
    for (const entry of entries || []) {
      const loaded = progressMap?.[entry.id] || 0;
      if (loaded >= entry.expectedSize) continue;

      const task = {
        id: entry.id,
        name: entry.name,
        expectedSize: entry.expectedSize,
        file: null,
        loaded,
        status: "incomplete",
        controller: null,
      };
      this.tasks.set(entry.id, task);
      this.callbacks.onUpdate(task);
    }
  }

  async submit(shareID, files) {
    this.shareID = shareID;
    const freshFiles = [];

    for (const file of files) {
      const existing = Array.from(this.tasks.values()).find(
        (t) =>
          t.status === "incomplete" &&
          t.name === file.name &&
          t.expectedSize === file.size,
      );
      if (existing) {
        existing.file = file;
        existing.status = "pending";
        this.callbacks.onUpdate(existing);
      } else {
        freshFiles.push(file);
      }
    }

    if (freshFiles.length > 0) {
      const newEntries = await registerFiles(shareID, freshFiles);
      newEntries.forEach((entry, i) => {
        const task = {
          id: entry.id,
          name: entry.name,
          expectedSize: entry.expectedSize,
          file: freshFiles[i],
          loaded: 0,
          status: "pending",
          controller: null,
        };
        this.tasks.set(task.id, task);
        this.callbacks.onUpdate(task);
      });
    }
    await this.drain();
  }

  async drain() {
    if (this.isDraining) return;
    this.isDraining = true;
    let didWork = false;

    for (const task of this.tasks.values()) {
      if (task.status !== "pending") continue;
      task.status = "uploading";
      task.controller = new AbortController();
      this.callbacks.onUpdate(task);

      try {
        await streamFile(
          this.shareID,
          task.id,
          task.file,
          (bytes) => {
            task.loaded = bytes;
            this.callbacks.onUpdate(task);
          },
          task.controller.signal,
        );

        task.status = "done";
        didWork = true;
      } catch (err) {
        const isAborted = task.controller && task.controller.signal.aborted;
        task.status = isAborted ? "incomplete" : "error";

        // Only trigger the error callback if it wasn't an intentional abort
        if (!isAborted && this.callbacks.onError) {
          this.callbacks.onError(
            `Failed to upload ${task.name}: ${err.message}`,
          );
        }
      } finally {
        task.controller = null;
        this.callbacks.onUpdate(task);
      }
    }
    this.isDraining = false;
    if (didWork) this.callbacks.onDrained();
  }

  async deleteTask(id) {
    const task = this.tasks.get(id);
    if (!task) return;
    if (task.controller) task.controller.abort();

    await fetch(`/api/shares/${this.shareID}/files/${id}`, {
      method: "DELETE",
    });
    this.tasks.delete(id);
    this.callbacks.onDelete(id);
    this.callbacks.onDrained();
  }

  resumeTask(id, file) {
    const task = this.tasks.get(id);
    if (!task) return;
    task.file = file;
    task.status = "pending";
    this.callbacks.onUpdate(task);
    this.drain().catch(() => {});
  }
}
